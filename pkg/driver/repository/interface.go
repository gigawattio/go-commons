package repository

import (
	"database/sql"
)

// RepositoryDriver defines the interface that must be implemented by
// data-store persistence drivers.
type RepositoryDriver interface {
	Save(value interface{}) (err error)
	SaveMultiple(values ...interface{}) (err error)

	Update(value interface{}, values interface{}) (rowsAffected int64, err error)
	UpdateSingle(value interface{}, values interface{}) (err error)

	Delete(value interface{}) (err error)
	DeleteMultiple(values ...interface{}) (err error)

	GetOrCreate(value interface{}) (created bool, err error)

	FirstWhere(value interface{}, query interface{}, args ...interface{}) (err error)
	FirstWhereOrder(value interface{}, order string, query interface{}, args ...interface{}) error

	LastWhere(value interface{}, query interface{}, args ...interface{}) (err error)
	LastWhereOrder(value interface{}, order string, query interface{}, args ...interface{}) error

	FindWhere(values interface{}, query interface{}, args ...interface{}) (err error)
	FindWhereOrder(values interface{}, order string, query interface{}, args ...interface{}) error
	FindWhereLimitOffset(values interface{}, limit int64, offset int64, query interface{}, args ...interface{}) (err error)
	FindWhereLimitOffsetOrder(values interface{}, limit int64, offset int64, order string, query interface{}, args ...interface{}) error

	// FindWhereRelated(values interface{}, model interface{}, relatedTo []interface{}, query interface{}, args ...interface{}) error
	FindRelated(model interface{}, relatedTo interface{}, foreignKeys ...string) (err error)
	AppendRelated(model interface{}, assocatedWith string, items ...interface{}) (err error)
	DeleteRelated(model interface{}, assocatedWith string, items ...interface{}) (err error)
	ClearRelated(model interface{}, assocatedWith string) (err error)
	CountRelated(model interface{}, assocatedWith string) (count int64, err error)

	CountWhere(query interface{}, args ...interface{}) (count int64, err error)

	RawRow(query string, args ...interface{}) (*sql.Row, error)
	RawRows(query string, args ...interface{}) (*sql.Rows, error)
	Raw(result interface{}, query string, args ...interface{}) (err error)

	Exec(query string, args ...interface{}) (err error)

	TableName(model interface{}) string
	DbName() (name string, err error)

	Close() (err error)
}
