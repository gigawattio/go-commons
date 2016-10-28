package repository

// General note:
//
// When using FirstWhere, LastWhere, FindWhere, etc.. if you are passing a
// struct object for the `query` param, make sure it is a pointer otherwise
// things will not work in the fashion you may intuitively expect.

import (
	"container/ring"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/gigawattio/go-commons/pkg/errorlib"

	"github.com/jinzhu/gorm"
)

// GormRepositoryDriver implements the `interfaces.RepositoryDriver` storage driver interface.
type GormRepositoryDriver struct {
	driverName        string
	connectionStrings *ring.Ring
	currentDb         *gorm.DB
	lock              sync.Mutex
}

func NewGormRepositoryDriver(driverName string, connectionStrings []string) (*GormRepositoryDriver, error) {
	driver := &GormRepositoryDriver{
		driverName:        driverName,
		connectionStrings: ring.New(len(connectionStrings)),
	}
	for _, connectionString := range connectionStrings {
		driver.connectionStrings.Value = connectionString
		driver.connectionStrings = driver.connectionStrings.Next()
	}
	log.Notice("Next connection string=%v", driver.connectionStrings.Value.(string))
	return driver, nil
}

func (driver *GormRepositoryDriver) Close() (err error) {
	driver.lock.Lock()
	defer driver.lock.Unlock()

	if driver.currentDb != nil {
		if err = driver.currentDb.Close(); err != nil {
			return
		}
	}
	return
}

func (driver *GormRepositoryDriver) db() (*gorm.DB, error) {
	driver.lock.Lock()
	defer driver.lock.Unlock()

	if driver.currentDb == nil {
		db, err := DbConnect(driver.driverName, driver.connectionStrings.Value.(string))
		driver.connectionStrings = driver.connectionStrings.Next()
		if err != nil {
			return nil, err
		}
		driver.currentDb = db
	}
	return driver.currentDb, nil
}

func (driver *GormRepositoryDriver) reset() {
	driver.lock.Lock()
	driver.currentDb = nil
	driver.lock.Unlock()
}

func isConnectionError(err *error) bool {
	errMsg := (*err).Error()
	if strings.HasPrefix(errMsg, "dial tcp ") && strings.HasSuffix(errMsg, ": connection refused") {
		return true
	}
	return false
}

func (driver *GormRepositoryDriver) withDb(fn func(db *gorm.DB) error) error {
	db, err := driver.db()
	if err != nil {
		return err
	}
	if err = fn(db); err != nil {
		if isConnectionError(&err) {
			driver.reset()
		}
		return err
	}
	return nil
}

func (driver *GormRepositoryDriver) withDbAssociation(model interface{}, associatedWith string, fn func(db *gorm.DB, association *gorm.Association) error) error {
	return driver.withDb(func(db *gorm.DB) error {
		var err error
		dbModel := db.Model(model)
		if err = dbModel.Error; err != nil {
			return err
		}
		association := dbModel.Association(associatedWith)
		if err = association.Error; err != nil {
			return err
		}
		if err = fn(db, association); err != nil {
			if isConnectionError(&err) {
				driver.reset()
			}
			return err
		}
		return nil
	})
}

type txFunc func(tx *gorm.DB) error

func (driver *GormRepositoryDriver) inTransaction(txFuncs ...txFunc) error {
	return driver.withDb(func(db *gorm.DB) (err error) {
		tx := db.Begin()
		if err = tx.Error; err != nil {
			err = errorlib.Merge([]error{err, tx.Rollback().Error})
			return
		}
		for _, fn := range txFuncs {
			if err = fn(tx); err != nil {
				err = errorlib.Merge([]error{err, tx.Rollback().Error})
				return
			}
		}
		if err = tx.Commit().Error; err != nil {
			err = errorlib.Merge([]error{err, tx.Rollback().Error})
			return
		}
		return
	})
}

func (driver *GormRepositoryDriver) Save(value interface{}) error {
	return driver.withDb(func(db *gorm.DB) (err error) {
		if err = db.Save(value).Error; err != nil {
			return
		}
		return
	})
}
func (driver *GormRepositoryDriver) SaveMultiple(values ...interface{}) error {
	if len(values) == 0 {
		return nil
	}
	return driver.inTransaction(func(tx *gorm.DB) (err error) {
		for _, value := range values {
			if err = tx.Save(value).Error; err != nil {
				return
			}
		}
		return
	})
}

// Update records matching `value`.
//
// Uses gorm's `UpdateColumns()' to avoid potential callbacks on related FK fields.
func (driver *GormRepositoryDriver) Update(value interface{}, values interface{}) (rowsAffected int64, err error) {
	err = driver.withDb(func(db *gorm.DB) (err error) {
		res := db.Model(value).UpdateColumns(values)
		if err = res.Error; err != nil {
			return
		}
		rowsAffected = res.RowsAffected
		return
	})
	if err != nil {
		err = fmt.Errorf("gorm driver: upd- %s", err)
	}
	return
}

// UpdateSingle updates a single row or throws an error.
//
// Uses gorm's `UpdateColumns()' to avoid potential callbacks on related FK fields.
func (driver *GormRepositoryDriver) UpdateSingle(value interface{}, values interface{}) error {
	return driver.inTransaction(func(tx *gorm.DB) (err error) {
		scope := tx.Model(value).UpdateColumns(values)
		if err = scope.Error; err != nil {
			return
		}
		if rowsAffected := scope.RowsAffected; rowsAffected != 1 {
			err = fmt.Errorf("gorm driver: upd1- 1 row should have been affected but instead %v rows were affected", rowsAffected)
			return
		}
		return
	})
}

func (driver *GormRepositoryDriver) Delete(value interface{}) error {
	return driver.withDb(func(db *gorm.DB) (err error) {
		err = db.Delete(value).Error
		if err != nil {
			err = fmt.Errorf("gorm driver: del- %s", err)
		}
		return
	})
}
func (driver *GormRepositoryDriver) DeleteMultiple(values ...interface{}) (err error) {
	if len(values) == 0 {
		return
	}
	if len(values) == 1 {
		// Guard against a list passed in without `...` since this could cause the
		// entire table contents to be deleted!
		if reflect.ValueOf(values[0]).Kind() == reflect.Slice {
			err = errors.New("gorm driver: dlm- invalid arguments to DeleteMultiple; did you forget the `...`?")
			return
		}
	}
	err = driver.inTransaction(func(tx *gorm.DB) (err error) {
		for i := range values {
			if err = tx.Delete(values[i]).Error; err != nil {
				return
			}
		}
		return
	})
	if err != nil {
		err = fmt.Errorf("gorm driver: dlm- %s", err)
		return
	}
	return
}

func (driver *GormRepositoryDriver) GetOrCreate(value interface{}) (created bool, err error) {
	err = driver.withDb(func(db *gorm.DB) (err error) {
		if err = db.Where(value).First(value).Error; err == gorm.ErrRecordNotFound {
			err = db.Create(value).Error
			created = true
		}
		return
	})
	if err != nil {
		err = fmt.Errorf("gorm driver: goc- %s", err)
		return
	}
	return
}

func (driver *GormRepositoryDriver) FirstWhere(value interface{}, query interface{}, args ...interface{}) error {
	return driver.withDb(func(db *gorm.DB) (err error) {
		err = db.Where(query, args...).First(value).Error
		if err != nil {
			err = fmt.Errorf("gorm driver: fw- %s", err)
			return
		}
		return
	})
}

func (driver *GormRepositoryDriver) FirstWhereOrder(value interface{}, order string, query interface{}, args ...interface{}) error {
	return driver.withDb(func(db *gorm.DB) (err error) {
		err = db.Where(query, args...).Order(order).First(value).Error
		if err != nil {
			err = fmt.Errorf("gorm driver: fwo- %s", err)
			return
		}
		return
	})
}

func (driver *GormRepositoryDriver) LastWhere(value interface{}, query interface{}, args ...interface{}) error {
	return driver.withDb(func(db *gorm.DB) (err error) {
		err = db.Where(query, args...).Last(value).Error
		if err != nil {
			err = fmt.Errorf("gorm driver: lw- %s", err)
			return
		}
		return
	})
}

func (driver *GormRepositoryDriver) LastWhereOrder(value interface{}, order string, query interface{}, args ...interface{}) error {
	return driver.withDb(func(db *gorm.DB) (err error) {
		err = db.Where(query, args...).Order(order).Last(value).Error
		if err != nil {
			err = fmt.Errorf("gorm driver: lwo- %s", err)
			return
		}
		return
	})
}

func (driver *GormRepositoryDriver) FindWhere(values interface{}, query interface{}, args ...interface{}) error {
	return driver.withDb(func(db *gorm.DB) (err error) {
		err = db.Where(query, args...).Find(values).Error
		if err != nil {
			err = fmt.Errorf("gorm driver: fndw- %s", err)
			return
		}
		return
	})
}

func (driver *GormRepositoryDriver) FindWhereOrder(values interface{}, order string, query interface{}, args ...interface{}) error {
	return driver.withDb(func(db *gorm.DB) (err error) {
		err = db.Where(query, args...).Order(order).Find(values).Error
		if err != nil {
			err = fmt.Errorf("gorm driver: fndwo- %s", err)
			return
		}
		return
	})
}

func (driver *GormRepositoryDriver) FindWhereLimitOffset(values interface{}, limit int64, offset int64, query interface{}, args ...interface{}) error {
	return driver.withDb(func(db *gorm.DB) (err error) {
		err = db.Order(`"id" DESC`).Limit(limit).Offset(offset).Where(query, args...).Find(values).Error
		if err != nil {
			err = fmt.Errorf("gorm driver: fwlo- %s", err)
			return
		}
		return
	})
}

func (driver *GormRepositoryDriver) FindWhereLimitOffsetOrder(values interface{}, limit int64, offset int64, order string, query interface{}, args ...interface{}) error {
	return driver.withDb(func(db *gorm.DB) (err error) {
		err = db.Order(order).Limit(limit).Offset(offset).Where(query, args...).Find(values).Error
		if err != nil {
			err = fmt.Errorf("gorm driver: fwloo- %s", err)
			return
		}
		return
	})
}

// func (driver *GormStorageDriver) FindWhereRelated(values interface{}, model interface{}, relatedTo []interface{}, query interface{}, args ...interface{}) error {
// 	return driver.withDb(func(db *gorm.DB) (err error) {
// 		err = db.Model(model).Related(relatedTo...).Where(query, args...).Find(values).Error
// 		if err != nil {
// 			err = fmt.Errorf("gorm driver: fndw- %s", err)
// 			return
// 		}
// 		return
// 	})
// }
func (driver *GormRepositoryDriver) FindRelated(model interface{}, relatedTo interface{}, foreignKeys ...string) error {
	return driver.withDb(func(db *gorm.DB) (err error) {
		err = db.Model(model).Related(relatedTo, foreignKeys...).Error
		if err != nil {
			err = fmt.Errorf("gorm driver: fnr- %s", err)
			return
		}
		return
	})
}
func (driver *GormRepositoryDriver) AppendRelated(model interface{}, associatedWith string, items ...interface{}) error {
	return driver.withDbAssociation(model, associatedWith, func(db *gorm.DB, association *gorm.Association) (err error) {
		err = association.Append(items...).Error
		if err != nil {
			err = fmt.Errorf("gorm driver: apr- %s", err)
			return
		}
		return
	})
}
func (driver *GormRepositoryDriver) DeleteRelated(model interface{}, associatedWith string, items ...interface{}) error {
	return driver.withDbAssociation(model, associatedWith, func(db *gorm.DB, association *gorm.Association) (err error) {
		err = association.Delete(items...).Error
		if err != nil {
			err = fmt.Errorf("gorm driver: dlr- %s", err)
			return
		}
		return
	})
}
func (driver *GormRepositoryDriver) ClearRelated(model interface{}, associatedWith string) error {
	return driver.withDbAssociation(model, associatedWith, func(db *gorm.DB, association *gorm.Association) (err error) {
		err = association.Clear().Error
		if err != nil {
			err = fmt.Errorf("gorm driver: upd- %s", err)
			return
		}
		return
	})
}
func (driver *GormRepositoryDriver) CountRelated(model interface{}, associatedWith string) (count int64, err error) {
	err = driver.withDbAssociation(model, associatedWith, func(db *gorm.DB, association *gorm.Association) (err error) {
		count = int64(association.Count())
		err = association.Error
		return
	})
	if err != nil {
		err = fmt.Errorf("gorm driver: cr- %s", err)
		return
	}
	return
}

func (driver *GormRepositoryDriver) CountWhere(query interface{}, args ...interface{}) (count int64, err error) {
	err = driver.withDb(func(db *gorm.DB) (err error) {
		err = db.Where(query, args...).Count(&count).Error
		return
	})
	if err != nil {
		err = fmt.Errorf("gorm driver: upd- %s", err)
		return
	}
	return
}

func (driver *GormRepositoryDriver) Exec(query string, args ...interface{}) error {
	return driver.withDb(func(db *gorm.DB) (err error) {
		if err = db.Exec(query, args...).Error; err != nil {
			return
		}
		if err != nil {
			err = fmt.Errorf("gorm driver: exe- %s", err)
			return
		}
		return
	})
}

func (driver *GormRepositoryDriver) TableName(model interface{}) (tableName string) {
	driver.withDb(func(db *gorm.DB) error {
		tableName = db.NewScope(model).TableName()
		return nil
	})
	return
}

func (driver *GormRepositoryDriver) DbName() (name string, err error) {
	/*err = driver.withDb(func(db *gorm.DB) error {
		name = db.CurrentDatabase()
		if name == "" {
			return errors.New("current database name is unknown")
		}
		return nil
	})
	if err != nil {
		return
	}*/
	err = errors.New("not implemented")
	return
}

func (driver *GormRepositoryDriver) Raw(result interface{}, query string, args ...interface{}) error {
	err := driver.withDb(func(db *gorm.DB) (err error) {
		res := db.Raw(query, args...)
		if err = res.Error; err != nil {
			return
		}

		var rows *sql.Rows
		if rows, err = res.Rows(); err != nil {
			return
		}
		defer rows.Close()

		switch result.(type) {
		// primitive types.
		case *bool:
			assign := result.(*bool)
			var x bool
			rows.Next()
			if err = rows.Scan(&x); err != nil {
				return
			}
			*assign = x

		case *int:
			assign := result.(*int)
			var x int
			for rows.Next() {
				if err = rows.Scan(&x); err != nil {
					return
				}
			}
			*assign = x

		case *int64:
			assign := result.(*int64)
			var x int64
			rows.Next()
			if err = rows.Scan(&x); err != nil {
				return
			}
			*assign = x

		case *byte:
			assign := result.(*byte)
			var x byte
			rows.Next()
			if err = rows.Scan(&x); err != nil {
				return
			}
			*assign = x

		case *string:
			assign := result.(*string)
			var x string
			rows.Next()
			if err = rows.Scan(&x); err != nil {
				return
			}
			*assign = x

		// slice types.
		case *[]bool:
			assign := result.(*[]bool)
			var x bool
			slice := []bool{}
			for rows.Next() {
				if err = rows.Scan(&x); err != nil {
					return
				}
				slice = append(slice, x)
			}
			*assign = slice

		case *[]int:
			assign := result.(*[]int)
			var x int
			slice := []int{}
			for rows.Next() {
				if err = rows.Scan(&x); err != nil {
					return
				}
				slice = append(slice, x)
			}
			*assign = slice

		case *[]int64:
			assign := result.(*[]int64)
			var x int64
			slice := []int64{}
			for rows.Next() {
				if err = rows.Scan(&x); err != nil {
					return
				}
				slice = append(slice, x)
			}
			*assign = slice

		case *[]byte:
			assign := result.(*[]byte)
			var x byte
			slice := []byte{}
			for rows.Next() {
				if err = rows.Scan(&x); err != nil {
					return
				}
				slice = append(slice, x)
			}
			*assign = slice

		case *[]string:
			assign := result.(*[]string)
			var x string
			slice := []string{}
			for rows.Next() {
				if err = rows.Scan(&x); err != nil {
					return
				}
				slice = append(slice, x)
			}
			*assign = slice

		// 2D-slice types.
		case *[][]bool:
			var (
				assign   = result.(*[][]bool)
				slice    = [][]bool{}
				out      []bool
				pointers []*bool
				ifaces   []interface{}
				ln       int
				cols     []string
			)
			for rows.Next() {
				if cols, err = rows.Columns(); err != nil {
					return
				}
				ln = len(cols)
				out = make([]bool, ln)
				pointers = make([]*bool, ln)
				ifaces = make([]interface{}, ln)
				for i := 0; i < ln; i++ { // ifaces destinations must be pointers.
					ifaces[i] = &pointers[i]
				}
				if err = rows.Scan(ifaces...); err != nil {
					return
				}
				for i := 0; i < ln; i++ {
					if pointers[i] != nil {
						out[i] = *pointers[i]
					}
				}
				slice = append(slice, out)
			}
			*assign = slice

		case *[][]int:
			var (
				assign   = result.(*[][]int)
				slice    = [][]int{}
				out      []int
				pointers []*int
				ifaces   []interface{}
				ln       int
				cols     []string
			)
			for rows.Next() {
				if cols, err = rows.Columns(); err != nil {
					return
				}
				ln = len(cols)
				out = make([]int, ln)
				pointers = make([]*int, ln)
				ifaces = make([]interface{}, ln)
				for i := 0; i < ln; i++ { // ifaces destinations must be pointers.
					ifaces[i] = &pointers[i]
				}
				if err = rows.Scan(ifaces...); err != nil {
					return
				}
				for i := 0; i < ln; i++ {
					if pointers[i] != nil {
						out[i] = *pointers[i]
					}
				}
				slice = append(slice, out)
			}
			*assign = slice

		case *[][]int64:
			var (
				assign   = result.(*[][]int64)
				slice    = [][]int64{}
				out      []int64
				pointers []*int64
				ifaces   []interface{}
				ln       int
				cols     []string
			)
			for rows.Next() {
				if cols, err = rows.Columns(); err != nil {
					return
				}
				ln = len(cols)
				out = make([]int64, ln)
				pointers = make([]*int64, ln)
				ifaces = make([]interface{}, ln)
				for i := 0; i < ln; i++ { // ifaces destinations must be pointers.
					ifaces[i] = &pointers[i]
				}
				if err = rows.Scan(ifaces...); err != nil {
					return
				}
				for i := 0; i < ln; i++ {
					if pointers[i] != nil {
						out[i] = *pointers[i]
					}
				}
				slice = append(slice, out)
			}
			*assign = slice

		case *[][]byte:
			var (
				assign   = result.(*[][]byte)
				slice    = [][]byte{}
				out      []byte
				pointers []*byte
				ifaces   []interface{}
				ln       int
				cols     []string
			)
			for rows.Next() {
				if cols, err = rows.Columns(); err != nil {
					return
				}
				ln = len(cols)
				out = make([]byte, ln)
				pointers = make([]*byte, ln)
				ifaces = make([]interface{}, ln)
				for i := 0; i < ln; i++ { // ifaces destinations must be pointers.
					ifaces[i] = &pointers[i]
				}
				if err = rows.Scan(ifaces...); err != nil {
					return
				}
				for i := 0; i < ln; i++ {
					if pointers[i] != nil {
						out[i] = *pointers[i]
					}
				}
				slice = append(slice, out)
			}
			*assign = slice

		case *[][]string:
			var (
				assign   = result.(*[][]string)
				slice    = [][]string{}
				out      []string
				pointers []*string
				ifaces   []interface{}
				ln       int
				cols     []string
			)
			for rows.Next() {
				if cols, err = rows.Columns(); err != nil {
					return
				}
				ln = len(cols)
				out = make([]string, ln)
				pointers = make([]*string, ln)
				ifaces = make([]interface{}, ln)
				for i := 0; i < ln; i++ { // ifaces destinations must be pointers.
					ifaces[i] = &pointers[i]
				}
				if err = rows.Scan(ifaces...); err != nil {
					return
				}
				for i := 0; i < ln; i++ {
					if pointers[i] != nil {
						out[i] = *pointers[i]
					}
				}
				slice = append(slice, out)
			}
			*assign = slice

		// map types.
		case *map[string]bool:
			assign := result.(*map[string]bool)
			if assign == nil || *assign == nil {
				*assign = map[string]bool{}
			}
			var columns []string
			if columns, err = rows.Columns(); err != nil {
				err = fmt.Errorf("gorm driver: getting columns from result T=%T rows: %s", result, err)
				return
			}
			values := make([]bool, len(columns))
			ifacesPtrs := make([]interface{}, len(columns))
			for i, l := 0, len(columns); i < l; i++ {
				ifacesPtrs[i] = &values[i]
			}
			for rows.Next() {
				if err = rows.Scan(ifacesPtrs...); err != nil {
					return
				}
				for i, column := range columns {
					(*assign)[column] = values[i]
				}
			}

		case *map[string]int:
			assign := result.(*map[string]int)
			if assign == nil || *assign == nil {
				*assign = map[string]int{}
			}
			var columns []string
			if columns, err = rows.Columns(); err != nil {
				err = fmt.Errorf("gorm driver: getting columns from result T=%T rows: %s", result, err)
				return
			}
			values := make([]int, len(columns))
			ifacesPtrs := make([]interface{}, len(columns))
			for i, l := 0, len(columns); i < l; i++ {
				ifacesPtrs[i] = &values[i]
			}
			for rows.Next() {
				if err = rows.Scan(ifacesPtrs...); err != nil {
					return
				}
				for i, column := range columns {
					(*assign)[column] = values[i]
				}
			}

		case *map[string]int64:
			assign := result.(*map[string]int64)
			if assign == nil || *assign == nil {
				*assign = map[string]int64{}
			}
			var columns []string
			if columns, err = rows.Columns(); err != nil {
				err = fmt.Errorf("getting columns from result T=%T rows: %s", result, err)
				return
			}
			values := make([]int64, len(columns))
			ifacesPtrs := make([]interface{}, len(columns))
			for i, l := 0, len(columns); i < l; i++ {
				ifacesPtrs[i] = &values[i]
			}
			for rows.Next() {
				if err = rows.Scan(ifacesPtrs...); err != nil {
					return
				}
				for i, column := range columns {
					(*assign)[column] = values[i]
				}
			}

		case *map[string]byte:
			assign := result.(*map[string]byte)
			if assign == nil || *assign == nil {
				*assign = map[string]byte{}
			}
			var columns []string
			if columns, err = rows.Columns(); err != nil {
				err = fmt.Errorf("getting columns from result T=%T rows: %s", result, err)
				return
			}
			values := make([]byte, len(columns))
			ifacesPtrs := make([]interface{}, len(columns))
			for i, l := 0, len(columns); i < l; i++ {
				ifacesPtrs[i] = &values[i]
			}
			for rows.Next() {
				if err = rows.Scan(ifacesPtrs...); err != nil {
					return
				}
				for i, column := range columns {
					(*assign)[column] = values[i]
				}
			}

		case *map[string][]byte:
			assign := result.(*map[string][]byte)
			if assign == nil || *assign == nil {
				*assign = map[string][]byte{}
			}
			var columns []string
			if columns, err = rows.Columns(); err != nil {
				err = fmt.Errorf("getting columns from result T=%T rows: %s", result, err)
				return
			}
			values := make([][]byte, len(columns))
			ifacesPtrs := make([]interface{}, len(columns))
			for i, l := 0, len(columns); i < l; i++ {
				ifacesPtrs[i] = &values[i]
			}
			for rows.Next() {
				if err = rows.Scan(ifacesPtrs...); err != nil {
					return
				}
				for i, column := range columns {
					(*assign)[column] = values[i]
				}
			}

		case *map[string]string:
			assign := result.(*map[string]string)
			if assign == nil || *assign == nil {
				*assign = map[string]string{}
			}
			var columns []string
			if columns, err = rows.Columns(); err != nil {
				err = fmt.Errorf("getting columns from result T=%T rows: %s", result, err)
				return
			}
			values := make([]string, len(columns))
			ifacesPtrs := make([]interface{}, len(columns))
			for i, l := 0, len(columns); i < l; i++ {
				ifacesPtrs[i] = &values[i]
			}
			for rows.Next() {
				if err = rows.Scan(ifacesPtrs...); err != nil {
					return
				}
				for i, column := range columns {
					(*assign)[column] = values[i]
				}
			}

		case *map[string]interface{}:
			assign := result.(*map[string]interface{})
			if assign == nil || *assign == nil {
				*assign = map[string]interface{}{}
			}
			var columns []string
			if columns, err = rows.Columns(); err != nil {
				err = fmt.Errorf("getting columns from result T=%T rows: %s", result, err)
				return
			}
			values := make([]interface{}, len(columns))
			ifacesPtrs := make([]interface{}, len(columns))
			for i, l := 0, len(columns); i < l; i++ {
				ifacesPtrs[i] = &values[i]
			}
			for rows.Next() {
				if err = rows.Scan(ifacesPtrs...); err != nil {
					return
				}
				for i, column := range columns {
					(*assign)[column] = values[i]
				}
			}

		// slice-map types.
		case *[]map[string]bool:
			assign := result.(*[]map[string]bool)
			if assign == nil || *assign == nil {
				*assign = []map[string]bool{}
			}
			var columns []string
			if columns, err = rows.Columns(); err != nil {
				err = fmt.Errorf("getting columns from result T=%T rows: %s", result, err)
				return
			}
			values := make([]bool, len(columns))
			ifacesPtrs := make([]interface{}, len(columns))
			for i, l := 0, len(columns); i < l; i++ {
				ifacesPtrs[i] = &values[i]
			}
			for rows.Next() {
				if err = rows.Scan(ifacesPtrs...); err != nil {
					return
				}
				row := map[string]bool{}
				for i, column := range columns {
					row[column] = values[i]
				}
				*assign = append(*assign, row)
			}

		case *[]map[string]int:
			assign := result.(*[]map[string]int)
			if assign == nil || *assign == nil {
				*assign = []map[string]int{}
			}
			var columns []string
			if columns, err = rows.Columns(); err != nil {
				err = fmt.Errorf("getting columns from result T=%T rows: %s", result, err)
				return
			}
			values := make([]int, len(columns))
			ifacesPtrs := make([]interface{}, len(columns))
			for i, l := 0, len(columns); i < l; i++ {
				ifacesPtrs[i] = &values[i]
			}
			for rows.Next() {
				if err = rows.Scan(ifacesPtrs...); err != nil {
					return
				}
				row := map[string]int{}
				for i, column := range columns {
					row[column] = values[i]
				}
				*assign = append(*assign, row)
			}

		case *[]map[string]int64:
			assign := result.(*[]map[string]int64)
			if assign == nil || *assign == nil {
				*assign = []map[string]int64{}
			}
			var columns []string
			if columns, err = rows.Columns(); err != nil {
				err = fmt.Errorf("getting columns from result T=%T rows: %s", result, err)
				return
			}
			values := make([]int64, len(columns))
			ifacesPtrs := make([]interface{}, len(columns))
			for i, l := 0, len(columns); i < l; i++ {
				ifacesPtrs[i] = &values[i]
			}
			for rows.Next() {
				if err = rows.Scan(ifacesPtrs...); err != nil {
					return
				}
				row := map[string]int64{}
				for i, column := range columns {
					row[column] = values[i]
				}
				*assign = append(*assign, row)
			}

		case *[]map[string]byte:
			assign := result.(*[]map[string]byte)
			if assign == nil || *assign == nil {
				*assign = []map[string]byte{}
			}
			var columns []string
			if columns, err = rows.Columns(); err != nil {
				err = fmt.Errorf("getting columns from result T=%T rows: %s", result, err)
				return
			}
			values := make([]byte, len(columns))
			ifacesPtrs := make([]interface{}, len(columns))
			for i, l := 0, len(columns); i < l; i++ {
				ifacesPtrs[i] = &values[i]
			}
			for rows.Next() {
				if err = rows.Scan(ifacesPtrs...); err != nil {
					return
				}
				row := map[string]byte{}
				for i, column := range columns {
					row[column] = values[i]
				}
				*assign = append(*assign, row)
			}

		case *[]map[string][]byte:
			assign := result.(*[]map[string][]byte)
			if assign == nil || *assign == nil {
				*assign = []map[string][]byte{}
			}
			var columns []string
			if columns, err = rows.Columns(); err != nil {
				err = fmt.Errorf("getting columns from result T=%T rows: %s", result, err)
				return
			}
			values := make([][]byte, len(columns))
			ifacesPtrs := make([]interface{}, len(columns))
			for i, l := 0, len(columns); i < l; i++ {
				ifacesPtrs[i] = &values[i]
			}
			for rows.Next() {
				if err = rows.Scan(ifacesPtrs...); err != nil {
					return
				}
				row := map[string][]byte{}
				for i, column := range columns {
					row[column] = values[i]
				}
				*assign = append(*assign, row)
			}

		case *[]map[string]string:
			assign := result.(*[]map[string]string)
			if assign == nil || *assign == nil {
				*assign = []map[string]string{}
			}
			var columns []string
			if columns, err = rows.Columns(); err != nil {
				err = fmt.Errorf("getting columns from result T=%T rows: %s", result, err)
				return
			}
			values := make([]string, len(columns))
			ifacesPtrs := make([]interface{}, len(columns))
			for i, l := 0, len(columns); i < l; i++ {
				ifacesPtrs[i] = &values[i]
			}
			for rows.Next() {
				if err = rows.Scan(ifacesPtrs...); err != nil {
					return
				}
				row := map[string]string{}
				for i, column := range columns {
					row[column] = values[i]
				}
				*assign = append(*assign, row)
			}

		case *[]map[string]interface{}:
			assign := result.(*[]map[string]interface{})
			if assign == nil || *assign == nil {
				*assign = []map[string]interface{}{}
			}
			var columns []string
			if columns, err = rows.Columns(); err != nil {
				err = fmt.Errorf("getting columns from result T=%T rows: %s", result, err)
				return
			}
			values := make([]interface{}, len(columns))
			ifacesPtrs := make([]interface{}, len(columns))
			for i, l := 0, len(columns); i < l; i++ {
				ifacesPtrs[i] = &values[i]
			}
			for rows.Next() {
				if err = rows.Scan(ifacesPtrs...); err != nil {
					return
				}
				row := map[string]interface{}{}
				for i, column := range columns {
					row[column] = values[i]
				}
				*assign = append(*assign, row)
			}

		default:
			log.Debug("gorm driver: unsupported result type: %T, falling back to gorm.Scan", result)
			if err = res.Scan(result).Error; err != nil {
				return
			}
			return
		}
		return
	})
	if err != nil {
		return fmt.Errorf("gorm driver: raw- %s", err)
	}
	return nil
}
