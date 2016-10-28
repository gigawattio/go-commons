package repository

import (
	"fmt"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	_ "github.com/lib/pq" // Imported for postgres-driver lib side effects.
)

const (
	FdbErrNotCommitted        = "1020 - not_committed"
	FdbErrPastVersion         = "1007 - past_version"
	FdbErrOnlineDdlInProgress = "Online DDL in progress for"
)

var (
	// FdbRetryLimit is the maximum number of retries that will be attempted for db
	// errors which match the criteria to be classified as an operation that can
	// safely be retried [until it succeeds].
	FdbRetryLimit = 100
)

func DbConnect(driver string, connectionString string) (*gorm.DB, error) {
	db, err := gorm.Open(driver, connectionString)
	if err != nil {
		return nil, err
	}

	if err = db.DB().Ping(); err != nil {
		return nil, err
	}
	db.DB().SetMaxIdleConns(10)
	db.DB().SetMaxOpenConns(20)

	// Disable pluralization of table names.
	db.SingularTable(true)

	db.LogMode(true)

	ConfigureAliveSupport(db)

	return db, nil
}

// ConfigureAliveSupport sets up `Alive' soft-deletion support for the provided
// db instance.
func ConfigureAliveSupport(db *gorm.DB) {
	AppendAliveToQuery := func(scope *gorm.Scope) {
		if !scope.Search.Unscoped && scope.HasColumn("alive") {
			sql := fmt.Sprintf(`%v.%v IS NOT NULL`, scope.QuotedTableName(), scope.Quote("alive"))
			scope.Search.Where(sql)
		}
	}

	db.Callback().Query().Before("gorm:query").Register("append_alive", AppendAliveToQuery)

	Delete := func(scope *gorm.Scope) {
		if !scope.HasError() {
			unscoped := scope.Search.Unscoped
			if !unscoped && scope.HasColumn("DeletedAt") {
				scope.Raw(
					fmt.Sprintf("UPDATE %v SET %v=%v %v",
						scope.QuotedTableName(),
						scope.Quote("deleted_at"),
						scope.AddToVars(gorm.NowFunc()),
						scope.CombinedConditionSql(),
					))
			} else if !unscoped && scope.HasColumn("Alive") {
				scope.Raw(
					fmt.Sprintf(`UPDATE %v SET %v=null %v`,
						scope.QuotedTableName(),
						scope.Quote("alive"),
						scope.CombinedConditionSql(),
					))
			} else {
				scope.Raw(fmt.Sprintf("DELETE FROM %v %v", scope.QuotedTableName(), scope.CombinedConditionSql()))
			}

			scope.Exec()
		}
	}

	db.Callback().Delete().Replace("gorm:delete", Delete)
}

// IsRetriableDbError checks an error to see if it is of the retriable foundationdb variety.
func IsRetriableDbError(err error) bool {
	if err != nil {
		str := err.Error()
		if strings.Contains(str, FdbErrNotCommitted) || strings.Contains(str, FdbErrPastVersion) || strings.Contains(str, FdbErrOnlineDdlInProgress) {
			return true
		}
	}
	return false
}

// DbExecWithRetry executes a statement on a `*gorm.DB` connection and checks
// for retriable errors.  If any are found, it will retry statement execution.
// See http://community.foundationdb.com/questions/42717/foundationdb-commit-aborted-1020-not-committed.html
// for more information about why this is sometimes necessary.
func DbExecWithRetry(db *gorm.DB, sql string, values ...interface{}) *gorm.DB {
	attemptNumber := 0
	var res0 *gorm.DB
	for {
		if res0 = db.Exec(sql, values...); res0.Error != nil {
			if IsRetriableDbError(res0.Error) {
				log.Info("ExecWithRetry: retriable error detected (failcount=%v err=%s), will retry query: %s", attemptNumber, res0.Error, sql)
				attemptNumber += 1
				time.Sleep(time.Duration(attemptNumber*10) * time.Millisecond)
				continue
			}
		}
		break
	}
	return res0
}

// DbFnWithRetry is just like ExecWithRetry except that it takes any
// function that produces a `*gorm.DB`.
func DbFnWithRetry(fn func() *gorm.DB) *gorm.DB {
	attemptNumber := 0
	var res0 *gorm.DB
	for {
		// Check if the max allowed retries has been exhausted.
		if FdbRetryLimit > 0 && attemptNumber > FdbRetryLimit {
			// Guard against res0 somehow being nil.
			if res0 == nil {
				res0 = &gorm.DB{
					Error: fmt.Errorf("oops, res0 is nil; is the retry limit > 0? If so, that's not allowed; FdbRetryLimit=%v", FdbRetryLimit),
				}
				return res0
			}
			res0.Error = fmt.Errorf("max allowed retries exceeded %v/%v: %v", attemptNumber, FdbRetryLimit, res0.Error)
			return res0
		}
		if res0 = fn(); res0 != nil && res0.Error != nil {
			if IsRetriableDbError(res0.Error) {
				log.Info("DbFnWithRetry: retriable error detected (failcount=%v err=%s); will retry", attemptNumber, res0.Error)
				attemptNumber += 1
				time.Sleep(time.Duration(attemptNumber*10) * time.Millisecond)
				continue
			}
		}
		if res0 == nil {
			res0 = &gorm.DB{
				Error: fmt.Errorf("oops, res0 is nil; is your fn returning a nil *gorm.DB? If so, that's not allowed; FdbRetryLimit=%v", FdbRetryLimit),
			}
			return res0
		}
		break
	}
	return res0
}
