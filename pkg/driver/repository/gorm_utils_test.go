package repository

import (
	"os"
	"regexp"
	"strings"

	"github.com/gigawattio/go-commons/pkg/driver/repository/gormlib"
	"github.com/gigawattio/go-commons/pkg/errorlib"
	"github.com/gigawattio/go-commons/pkg/oslib"
	"github.com/gigawattio/go-commons/pkg/testlib"

	"github.com/jinzhu/gorm"
	"github.com/lib/pq"
)

func DropSchema(driver string, connectionStrings []string, schemaToDrop string) error {
	var err error = errorlib.Error("db connection strings were all empty")
	for _, connectionString := range connectionStrings {
		// Alter dbname to connect to in order to guard against scenario where the
		// connected schema is being dropped.
		if strings.Contains(connectionString, "dbname="+schemaToDrop) {
			connectionString = strings.TrimSpace(regexp.MustCompile("dbname=[^ ]+").ReplaceAllString(connectionString, "") + " dbname=" + os.Getenv("USER"))
		}
		///////////////////////////////////////////////////////////////////////////
		// NB: Commented out for now because this could easily do more harm than //
		//     good!                                                             //
		// // Kill all active server connections.
		// var db1 *gorm.DB
		// if db1, err = gormlib.DbConnect(connectionString); err != nil {
		// 	return errorlib.Errorf("error getting database connection #1: %s", err)
		// }
		// defer db1.Close()
		// res1 := gormlib.DbExecWithRetry(db1, `ALTER SERVER KILL SESSION ALL`)
		// if err = res1.Error; err == nil {
		// 	return errorlib.Error("expected an error to result from killing all sessions, but no error was produced!")
		// }
		// Drop the schema.
		///////////////////////////////////////////////////////////////////////////
		var db2 *gorm.DB
		if db2, err = DbConnectForTesting(driver, connectionString); err != nil {
			err = errorlib.Errorf("error getting database connection #2: %s", err)
			continue
		}
		defer db2.Close()
		if driver == "foundation" {
			res2 := gormlib.DbExecWithRetry(db2, `DROP SCHEMA IF EXISTS "`+schemaToDrop+`" CASCADE`)
			if err = res2.Error; err != nil {
				return errorlib.Errorf("gormlib.DbExecWithRetry: %s", err)
			}
		} else {
			if err = db2.Exec(`DROP DATABASE IF EXISTS "` + schemaToDrop + `"`).Error; err != nil { //&& !strings.HasSuffix(err.Error(), "does not exist")
				return errorlib.Errorf("Dropping database=%v: %s", schemaToDrop, err)
			}
		}
		return nil
	}
	return err
}

type SchemaInitializerFn func(driver string, db *gorm.DB) error

func PopulateSchema(driver string, dbConnectionStrings []string, schemaInitializer SchemaInitializerFn) error {
	var err error = errorlib.Error("db connection strings were all empty")
	for _, dbConnectionString := range dbConnectionStrings {
		var db *gorm.DB
		db, err = func(dbConnectionString string) (*gorm.DB, error) {
			db, err := DbConnectForTesting(driver, dbConnectionString)
			if err != nil && testlib.IsRunningTests() {
				if driver == "postgres" && strings.HasSuffix(err.Error(), " does not exist") {
					// Attempt to detect desired db name and automatically create it.
					connectionString2 := regexp.MustCompile("dbname=[^ ]+").ReplaceAllString(dbConnectionString, "dbname="+os.Getenv("USER"))
					db2, err2 := DbConnectForTesting(driver, connectionString2)
					if err2 != nil {
						return nil, errorlib.Errorf("connecting to db: %s", err2)
					}
					defer db2.Close()
					dbName := regexp.MustCompile(`^[^"]+"|"[^"]+$`).ReplaceAllString(err.Error(), "")
					if err2 = db2.Exec(`CREATE DATABASE "` + dbName + `"`).Error; err2 != nil {
						return nil, errorlib.Errorf("automatic test db creation failed: %s", err2)
					}
					// dbName := regexp.MustCompile(`^["]*"([^"]+)".*$`).FindStringSubmatch(err.Error())
					// if err2 = db2.Exec(`CREATE DATABASE "` + dbName[0] + `"`).Error; err2 != nil {
					// 	return errorlib.Errorf("automatic test db creation failed: %s", err2)
					// }
					if db, err = DbConnectForTesting(driver, dbConnectionString); err != nil {
						return nil, errorlib.Errorf("db connection still failed after successful test db creation: %s", err)
					}
				} else {
					return nil, errorlib.Errorf("PopulateSchema failed to connect to db: %s", err)
				}
			} else if err != nil {
				return nil, err
			}
			return db, nil
		}(dbConnectionString)
		if err != nil {
			continue
		}
		defer db.Close()
		if err := schemaInitializer(driver, db); err != nil {
			return errorlib.Errorf("PopulateSchema failed to initialize schema: %s", err)
		}
		return nil
	}
	return err
}

func RecreatePaths(paths ...string) error {
	for _, path := range paths {
		// Clear out the temp directory.
		if err := oslib.BashCmdf("rm -rf %[1]s && mkdir -p %[1]s", path).Run(); err != nil {
			return err
		}
	}
	return nil
}

// CompleteReset
func CompleteReset(driver string, dbConnectionStrings []string, schemaInitializer SchemaInitializerFn, resetPaths ...string) error {
	if len(dbConnectionStrings) == 0 {
		return errorlib.Error("Received empty db connection strings slice")
	}
	currentSchema := testlib.CurrentRunningTest()
	// Sanity check db connection strings.
	for _, dbConnectionString := range dbConnectionStrings {
		if !strings.Contains(dbConnectionString, "dbname=Test") {
			return errorlib.Errorf(`Refusing to perform reset because dbConnectionString doesn't contain "dbname=[tT]est", actual dbConnectionString=%s`, dbConnectionString)
		}
		if !strings.Contains(dbConnectionString, currentSchema) {
			return errorlib.Errorf(`Refusing to perform reset: test schema "%s" missing from db connection string "%s"`, currentSchema, dbConnectionString)
		}
	}
	if err := RecreatePaths(resetPaths...); err != nil {
		return errorlib.Errorf("Failed to reset paths: %s, err=%s", resetPaths, err)
	}
	// Nuke any pre-existing items in the "testawatt" schema.
	if err := DropSchema(driver, dbConnectionStrings, currentSchema); err != nil {
		return err
	}
	if err := PopulateSchema(driver, dbConnectionStrings, schemaInitializer); err != nil {
		return err
	}
	return nil
}

// DbConnectForTesting is only to be used during testing.  Attempts to
// automatically recover from specific error classes.
func DbConnectForTesting(driver string, connectionString string) (*gorm.DB, error) {
	if !testlib.IsRunningTests() {
		panic("DbConnectForTesting is only to be used inside unit-tests.  It could result in security issues if used elsewhere.")
	}
	var (
		setParam = func(param string, value string) {
			delimiter := "&"
			switch driver {
			case "postgres":
				delimiter = " "
			}
			connectionString = strings.Trim(regexp.MustCompile(param+`=[^`+delimiter+`]+`).ReplaceAllString(connectionString, "")+delimiter+param+"="+value, delimiter)
		}
		errHandlers = []struct {
			Expr  string
			Apply func(appliedCount int)
			Count int
		}{
			{
				Expr:  pq.ErrSSLNotSupported.Error(),
				Apply: func(_ int) { setParam("sslmode", "disable") },
			},
			{
				Expr: pq.ErrCouldNotDetectUsername.Error(),
				Apply: func(appliedCount int) {
					switch appliedCount {
					case 0:
						setParam("user", os.Getenv("USER"))
					default:
						setParam("user", "postgres")
					}
				},
			},
			{
				Expr:  `role ".*" does not exist`,
				Apply: func(_ int) { setParam("user", os.Getenv("USER")) },
			},
			{
				Expr:  `role "` + os.Getenv("USER") + `" does not exist`,
				Apply: func(_ int) { setParam("user", "postgres") },
			},
		}
		db  *gorm.DB
		err error
	)

	const maxNumApplications = 3

	for {
		if db, err = gormlib.DbConnect(driver, connectionString); err == nil {
			break
		}
		var appliedAny bool
		for _, errHandler := range errHandlers {
			if regexp.MustCompile(errHandler.Expr).MatchString(err.Error()) && errHandler.Count < maxNumApplications {
				errHandler.Apply(errHandler.Count)
				errHandler.Count++
				appliedAny = true
			}
		}
		if !appliedAny {
			return nil, err
		}
	}
	return db, nil
}
