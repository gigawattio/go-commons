package repository

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gigawattio/go-commons/pkg/driver/repository/gormlib"
	"github.com/gigawattio/go-commons/pkg/testlib"

	log "github.com/Sirupsen/logrus"
	"github.com/jinzhu/gorm"
)

type (
	Tag struct {
		Id       int64
		Name     string    `gorm:"not null;unique;"`
		MyDatums []MyDatum `gorm:"many2many:my_datum_tag;"`
	}

	MyDatum struct {
		Id         int64
		Name       string    `gorm:"not null;unique;"`
		HomePlanet string    `gorm:"type:varchar(255);"`
		Metadata   string    `gorm:"type:text"`
		CreatedAt  time.Time `gorm:"type:timestamp without time zone;not null;DEFAULT:current_timestamp;"`
		UpdatedAt  time.Time `gorm:"type:timestamp without time zone;not null;DEFAULT:current_timestamp;" gorm:"update_time_stamp_when_update:yes;"`
		Tags       []Tag     `gorm:"many2many:my_datum_tag;"`
	}

	MyDatumTag struct {
		MyDatumId int64 `gorm:"type:bigint REFERENCES \"my_datum\" (\"id\");not null;"`
		TagId     int64 `gorm:"type:bigint REFERENCES \"tag\" (\"id\");not null;"`
	}
)

var (
	dbDriverName        = "postgres"
	dbConnectionStrings = []string{"dbname=TestGigawattIO"}

	entities = []interface{}{
		&Tag{},
		&MyDatum{},
		&MyDatumTag{},
	}
)

func init() {
	if dbDriverNameOverride := os.Getenv("DB_DRIVER"); len(dbDriverNameOverride) > 0 {
		log.Infof("dbDriverName override detected, old-value=%q new-value=%q", dbDriverName, dbDriverNameOverride)
		dbDriverName = dbDriverNameOverride
	}
	if dbConnectionStringsOverride := strings.Split(os.Getenv("DB_CONNECTION_STRINGS"), ","); len(os.Getenv("DB_CONNECTION_STRINGS")) > 0 { // NB: split("") => []string{""} with len=1.
		log.Infof("dbConnectionStrings override detected, old-value=%v new-value=%v", dbConnectionStrings, dbConnectionStringsOverride)
		dbConnectionStrings = dbConnectionStringsOverride
	}
}

func initSchema(_ string, db *gorm.DB) error {
	var preExistingTagsTable bool
	if db.HasTable(&MyDatumTag{}) {
		preExistingTagsTable = true
	}

	for _, entity := range entities {
		res0 := gormlib.DbFnWithRetry(func() *gorm.DB { return db.AutoMigrate(entity) })
		if res0.Error != nil {
			return res0.Error
		}
	}

	// Custom override when tags table was just created.
	// This is a way to get proper foreign-keys on the tags table.
	if !preExistingTagsTable {
		fmt.Printf("Recreating tags table..")
		if err := db.DropTable(&MyDatumTag{}).Error; err != nil {
			return err
		}
		if err := db.AutoMigrate(&MyDatumTag{}).Error; err != nil {
			return err
		}
	}

	type UniqueIndex struct {
		model   interface{}
		name    string
		columns []string
	}
	uniqueIndexes := []UniqueIndex{
		UniqueIndex{
			model:   &MyDatum{},
			name:    "unique_my_datum",
			columns: []string{"name"},
		},
		UniqueIndex{
			model:   &Tag{},
			name:    "unique_tag",
			columns: []string{"name"},
		},
		UniqueIndex{
			model:   &MyDatumTag{},
			name:    "unique_my_datum_tag",
			columns: []string{"my_datum_id", "tag_id"},
		},
	}
	for _, uidx := range uniqueIndexes {
		res0 := gormlib.DbFnWithRetry(func() *gorm.DB { return db.Model(uidx.model).AddUniqueIndex(uidx.name, uidx.columns...) })
		if err := res0.Error; err != nil {
			return err
		}
	}
	return nil
}

var dbNameExpr = regexp.MustCompile(`dbname=[^ ]+`)

func reset(t *testing.T, dbDriverName string, dbConnectionStrings []string) (*GormRepositoryDriver, func()) {
	patchedDbConnectionStrings := make([]string, len(dbConnectionStrings))
	for i, dbConnectionString := range dbConnectionStrings {
		// fmt.Fprintf(os.Stderr, "BEFORE: %s (AND crt=%v)\n", dbConnectionString, testlib.CurrentRunningTest())
		patchedDbConnectionStrings[i] = strings.TrimSpace(dbNameExpr.ReplaceAllString(dbConnectionString, "") + " dbname=" + testlib.CurrentRunningTest())
		// fmt.Fprintf(os.Stderr, "AFTER : %s\n", patchedDbConnectionStrings[i])
	}

	if err := CompleteReset(dbDriverName, patchedDbConnectionStrings, initSchema); err != nil {
		t.Fatalf("Fatal error during reset: %s", err)
	}
	driver, err := NewGormRepositoryDriver(dbDriverName, patchedDbConnectionStrings)
	if err != nil {
		t.Fatal(err)
	}
	driver.ConnectorFunc = DbConnectForTesting

	cleanupFunc := func() {
		if !t.Failed() {
			if err := driver.Close(); err != nil {
				t.Fatal(err)
			}
			driver, err := NewGormRepositoryDriver(dbDriverName, dbConnectionStrings)
			if err != nil {
				t.Fatal(err)
			}
			driver.ConnectorFunc = DbConnectForTesting
			if err := driver.Exec(`DROP DATABASE "` + testlib.CurrentRunningTest() + `"`); err != nil {
				t.Fatalf("Error during cleanup: %s", err)
			}
			if err := driver.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
	return driver, cleanupFunc
}

func TestGetOrCreate(t *testing.T) {
	driver, cleanupFunc := reset(t, dbDriverName, dbConnectionStrings)
	defer cleanupFunc()
	char1 := &MyDatum{Name: "Turd Ferguson"}
	if _, err := driver.GetOrCreate(char1); err != nil {
		t.Fatal(err)
	}
	if char1.Id == 0 {
		t.Fatalf("Record id is 0 after invoking GetOrCreate, record=%v", char1)
	}
	char2 := &MyDatum{Name: "Turd Ferguson"}
	if _, err := driver.GetOrCreate(char2); err != nil {
		t.Fatal(err)
	}
	if char2.Id != char1.Id {
		t.Fatalf("Expected second record id to match first, but char1.id=%v and char2.id=%v", char1.Id, char2)
	}
	char3 := &MyDatum{Name: "Turd Ferguson"}
	if err := driver.Save(char3); err == nil || !regexp.MustCompile(`duplicate key.*violates unique constraint`).MatchString(strings.ToLower(err.Error())) {
		t.Fatalf("Expected error matching `duplicate key.*violates unique constraint' error but instead found err=%s", err)
	}
}

func TestUpdateSingle(t *testing.T) {
	driver, cleanupFunc := reset(t, dbDriverName, dbConnectionStrings)
	defer cleanupFunc()
	commonMeta := "single"
	iggy := &MyDatum{Name: "Iggy Azalea", Metadata: commonMeta}
	if err := driver.Save(iggy); err != nil {
		t.Fatal(err)
	}
	// Success case.
	{
		newName := "not!"
		if err := driver.UpdateSingle(iggy, MyDatum{Name: newName}); err != nil {
			t.Fatal(err)
		}
	}
	// Case which should fail.
	{
		i2 := &MyDatum{Name: "i2", Metadata: commonMeta}
		if err := driver.Save(i2); err != nil {
			t.Fatal(err)
		}
		if err := driver.UpdateSingle(&MyDatum{Metadata: commonMeta}, MyDatum{HomePlanet: "Venus"}); err == nil {
			t.Fatalf(`UpdateSingle should have failed for MyDatum{Metadata: "%s"} but err=%v`, commonMeta, err)
		}
	}
}

func TestMultiDelete(t *testing.T) {
	driver, cleanupFunc := reset(t, dbDriverName, dbConnectionStrings)
	defer cleanupFunc()
	testCases := [][]string{
		[]string{"a"},
		[]string{"a", "b"},
		[]string{"a", "b", "c"},
		[]string{"a", "b", "c", "d"},
		[]string{"a", "b", "c", "d", "e"},
		[]string{"a", "b", "c", "d", "e", "f"},
		[]string{"a", "b", "c", "d", "e", "f", "g"},
		[]string{"a", "b", "c", "d", "e", "f", "g", "h"},
		[]string{"a", "b", "c", "d", "e", "f", "g", "h", "i"},
		[]string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"},
		[]string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"},
		[]string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"},
		[]string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m"},
	}
	for _, testCase := range testCases {
		records := []*MyDatum{}
		for _, fragment := range testCase {
			d := &MyDatum{Name: "Turd Ferguson #" + fragment}
			if _, err := driver.GetOrCreate(d); err != nil {
				t.Fatalf("testCase=%+v failed to store record=%+v: %s", testCase, *d, err)
			}
			records = append(records, d)
		}
		// Convert to slice of interface.
		ifaces := make([]interface{}, len(records))
		for i := range records {
			ifaces[i] = records[i]
		}
		if err := driver.DeleteMultiple(ifaces...); err != nil {
			t.Fatalf("testCase=%+v multi-delete failed: %s", testCase, err)
		}
		// Test that improper usage generates an error.
		if err := driver.DeleteMultiple(ifaces); err == nil {
			t.Fatalf("testCase=%+v improper use of multi-delete succeeded when it should have failed", testCase)
		}
	}
}

func TestM2m(t *testing.T) {
	driver, cleanupFunc := reset(t, dbDriverName, dbConnectionStrings)
	defer cleanupFunc()

	// Create and store some taggable items.
	myDatums := []interface{}{}
	for i := 0; i < 10; i++ {
		myDatums = append(myDatums, &MyDatum{Name: fmt.Sprintf("m2m_test-%v", i)})
	}
	if err := driver.SaveMultiple(myDatums...); err != nil {
		t.Fatal(err)
	}

	// Create and store some tags.
	tagIfaces := []interface{}{}
	tagStrings := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "1", "2", "3", "4", "5"}
	for _, name := range tagStrings {
		tagIfaces = append(tagIfaces, &Tag{Name: "tag-" + name})
	}
	if err := driver.SaveMultiple(tagIfaces...); err != nil {
		t.Fatal(err)
	}

	// Add datum-tag relations.
	{
		i := 0
		for _, iface := range myDatums {
			myDatum := iface.(*MyDatum)
			tagWith := make([]interface{}, 3)
			for j := 0; j < 3; j++ {
				i += j
				if i >= len(tagIfaces) {
					i = 0
				}
				tagWith[j] = tagIfaces[i]
			}
			if err := driver.AppendRelated(myDatum, "Tags", tagWith...); err != nil {
				t.Fatalf("Error appending assocations=%+v to myDatum=%+v: %s", tagWith, myDatum, err)
			}
			i++
		}
	}

	// Verify association counts.
	for _, iface := range myDatums {
		myDatum := iface.(*MyDatum)
		count, err := driver.CountRelated(myDatum, "Tags")
		if err != nil {
			t.Fatalf("Unexpected error getting tag count or myDatum=%+v: %s", myDatum, err)
		}
		if count != 3 {
			t.Fatalf("Expected tag count for myDatum=%+v to be 3, but instead actual count=%v", myDatum, count)
		}
	}

	// Query datum-tag relations.
	{
		myDatum := myDatums[0].(*MyDatum)
		tags := []Tag{}
		if err := driver.FindRelated(myDatum, &tags, "Tags"); err != nil {
			t.Fatalf("Unexpected error getting tags for myDatum=%+v: %s", myDatum, err)
		}
		if actual := len(tags); actual != 3 {
			t.Fatalf("Expected 3 tags to be returned for myDatum=%+v but instead got %v", myDatum, actual)
		}
	}
}

func TestRawRow(t *testing.T) {
	driver, cleanupFunc := reset(t, dbDriverName, dbConnectionStrings)
	defer cleanupFunc()

	n := int64(49)
	recs := make([]interface{}, n)
	for i := int64(0); i < n; i++ {
		recs[i] = &MyDatum{
			Name:       fmt.Sprintf("Name%v", i),
			HomePlanet: "Zurg",
		}
	}
	if err := driver.SaveMultiple(recs...); err != nil {
		t.Fatalf("Problem saving n=%v MyDatum records: %s", n, err)
	}
	row, err := driver.RawRow(`SELECT "id" FROM "my_datum" ORDER BY "id" DESC LIMIT 1`)
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := row.Scan(&id); err != nil {
		t.Fatal(err)
	}
	if expected, actual := n, id; actual != expected {
		t.Fatalf("Expected id=%v but actual=%v", expected, actual)
	}
}

func TestRawRows(t *testing.T) {
	driver, cleanupFunc := reset(t, dbDriverName, dbConnectionStrings)
	defer cleanupFunc()

	n := int64(49)
	recs := make([]interface{}, n)
	for i := int64(0); i < n; i++ {
		recs[i] = &MyDatum{
			Name:       fmt.Sprintf("Name%v", i),
			HomePlanet: "Zurg",
		}
	}
	if err := driver.SaveMultiple(recs...); err != nil {
		t.Fatalf("Problem saving n=%v MyDatum records: %s", n, err)
	}
	rows, err := driver.RawRows(`SELECT "id" FROM "my_datum"`)
	if err != nil {
		t.Fatal(err)
	}

	{
		cols, err := rows.Columns()
		if err != nil {
			t.Fatal(err)
		}
		if expected, actual := 1, len(cols); actual != expected {
			t.Fatalf("Expected len(cols)=%v but actual=%v", expected, actual)
		}
	}

	ids := []int64{}
	for rows.Next() {
		ids = append(ids, -1)
		if err := rows.Scan(&ids[len(ids)-1]); err != nil {
			t.Fatal(err)
		}
	}

	if expected, actual := n, int64(len(ids)); actual != expected {
		t.Fatalf("Expected len(ids)=%v but actaul=%v", expected, actual)
	}

	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRawScans(t *testing.T) {
	driver, cleanupFunc := reset(t, dbDriverName, dbConnectionStrings)
	defer cleanupFunc()

	// Ensure bad/invalid/mismatched queries emit errors.
	{
		var res []int64
		if err := driver.Raw(&res, "SELECT 1, 2"); err == nil {
			t.Errorf("Expected an error for bad query but err was nil")
		}
	}

	// bool
	{
		var (
			stmt     = "SELECT true, false, true, false, false"
			expected = [][]bool{[]bool{true, false, true, false, false}}
			res      [][]bool
		)
		if err := driver.Raw(&res, stmt); err != nil {
			t.Errorf("Expected query to succed but got err=%s", err)
		}
		if ln := len(res); ln != 1 {
			t.Fatalf("Expected len(res)=1 but actual=%v", ln)
		}
		if exp, actual := len(expected[0]), len(res[0]); actual != exp {
			t.Fatalf("Expected len(res[0])=%v but actual=%v", exp, actual)
		}
		if exp, actual := fmt.Sprintf("%+v", expected), fmt.Sprintf("%+v", res); actual != exp {
			t.Fatalf("Expected res=%v but actual=%v", exp, actual)
		}
	}

	// int
	{
		var (
			stmt     = "SELECT 1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024"
			expected = [][]int{[]int{1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024}}
			res      [][]int
		)
		if err := driver.Raw(&res, stmt); err != nil {
			t.Errorf("Expected query to succed but got err=%s", err)
		}
		if ln := len(res); ln != 1 {
			t.Fatalf("Expected len(res)=1 but actual=%v", ln)
		}
		if exp, actual := len(expected[0]), len(res[0]); actual != exp {
			t.Fatalf("Expected len(res[0])=%v but actual=%v", exp, actual)
		}
		if exp, actual := fmt.Sprintf("%+v", expected), fmt.Sprintf("%+v", res); actual != exp {
			t.Fatalf("Expected res=%v but actual=%v", exp, actual)
		}
	}

	// int64
	{
		var (
			stmt     = "SELECT 1, 2, 4, 8"
			expected = [][]int64{[]int64{1, 2, 4, 8}}
			res      [][]int64
		)
		if err := driver.Raw(&res, stmt); err != nil {
			t.Errorf("Expected query to succed but got err=%s", err)
		}
		if ln := len(res); ln != 1 {
			t.Fatalf("Expected len(res)=1 but actual=%v", ln)
		}
		if exp, actual := len(expected[0]), len(res[0]); actual != exp {
			t.Fatalf("Expected len(res[0])=%v but actual=%v", exp, actual)
		}
		if exp, actual := fmt.Sprintf("%+v", expected), fmt.Sprintf("%+v", res); actual != exp {
			t.Fatalf("Expected res=%v but actual=%v", exp, actual)
		}
	}

	// byte
	{
		var (
			stmt     = "SELECT 32, 33"
			expected = [][]byte{[]byte{32, 33}}
			res      [][]byte
		)
		if err := driver.Raw(&res, stmt); err != nil {
			t.Errorf("Expected query to succed but got err=%s", err)
		}
		if ln := len(res); ln != 1 {
			t.Fatalf("Expected len(res)=1 but actual=%v", ln)
		}
		if exp, actual := len(expected[0]), len(res[0]); actual != exp {
			t.Fatalf("Expected len(res[0])=%v but actual=%v", exp, actual)
		}
		if exp, actual := fmt.Sprintf("%+v", expected), fmt.Sprintf("%+v", res); actual != exp {
			t.Fatalf("Expected res=%v but actual=%v", exp, actual)
		}
	}

	// string
	{
		var (
			stmt     = "SELECT 'a', 'b'"
			expected = [][]string{[]string{"a", "b"}}
			res      [][]string
		)
		if err := driver.Raw(&res, stmt); err != nil {
			t.Errorf("Expected query to succed but got err=%s", err)
		}
		if ln := len(res); ln != 1 {
			t.Fatalf("Expected len(res)=1 but actual=%v", ln)
		}
		if exp, actual := len(expected[0]), len(res[0]); actual != exp {
			t.Fatalf("Expected len(res[0])=%v but actual=%v", exp, actual)
		}
		if exp, actual := fmt.Sprintf("%+v", expected), fmt.Sprintf("%+v", res); actual != exp {
			t.Fatalf("Expected res=%v but actual=%v", exp, actual)
		}
	}
}

func TestRawQueries(t *testing.T) {
	driver, cleanupFunc := reset(t, dbDriverName, dbConnectionStrings)
	defer cleanupFunc()

	n := 50
	recs := make([]interface{}, n)
	for i := 0; i < n; i++ {
		recs[i] = &MyDatum{
			Name:       fmt.Sprintf("Name%v", i),
			HomePlanet: "Zurg",
		}
	}
	if err := driver.SaveMultiple(recs...); err != nil {
		t.Fatalf("Problem saving n=%v MyDatum records: %s", n, err)
	}

	{
		var result struct {
			Id int
		} // Any non-primitive structure will do.
		err := driver.Raw(&result, `SELECT MAX("id") "id" FROM "my_datum"`)
		if err != nil {
			t.Errorf("Unexpeted error running strut population raw query: %s", err)
		}
		if result.Id != n {
			t.Errorf("Expected result.Id=%v but actual=%v", n, result.Id)
		}
	}

	{
		var result int
		if err := driver.Raw(&result, `SELECT MAX("id") FROM "my_datum"`); err != nil {
			t.Fatal(err)
		}
		if result < n {
			t.Errorf("Expected result >= %v, but actual result=%v", n, result)
		}
	}

	{
		var result []int
		if err := driver.Raw(&result, `SELECT "id" FROM "my_datum"`); err != nil {
			t.Fatal(err)
		}
		if l := len(result); l != n {
			t.Errorf("Expected result len=%v, but actual result len=%v", n, l)
		}
	}

	{
		var result map[string]string
		if err := driver.Raw(&result, `SELECT "name" FROM "my_datum" ORDER BY "id" ASC LIMIT 1`); err != nil {
			t.Fatal(err)
		}

		if l := len(result); l != 1 {
			t.Errorf("Expected result len=1, but actual result len=%v", l)
		} else if _, ok := result["name"]; !ok {
			t.Errorf("Expected result map to contain 'name', but the key wasn't found (result=%+v)", result)
		} else if name, _ := result["name"]; name != "Name0" {
			t.Errorf("Expected result['name'] = 'Name0', but instead actual=%v", name)
		}
	}

	{
		var result map[string][]byte
		if err := driver.Raw(&result, `SELECT "name" FROM "my_datum" ORDER BY "id" ASC LIMIT 1`); err != nil {
			t.Fatal(err)
		}

		if l := len(result); l != 1 {
			t.Errorf("Expected result len=1, but actual result len=%v", l)
		} else if _, ok := result["name"]; !ok {
			t.Errorf("Expected result map to contain 'name', but the key wasn't found (result=%+v)", result)
		} else if name, _ := result["name"]; string(name) != "Name0" {
			t.Errorf("Expected result['name'] = 'Name0', but instead actual bytes=%v string=%v", name, string(name))
		}
	}

	{
		var result map[string]interface{}
		if err := driver.Raw(&result, `SELECT "id", "name", "home_planet" FROM "my_datum" ORDER BY "id" ASC LIMIT 1`); err != nil {
			t.Fatal(err)
		}

		if l := len(result); l != 3 {
			t.Errorf("Expected result len=2, but actual result len=%v (result=%+v)", l, result)
		}

		// Validate "id".
		if _, ok := result["id"]; !ok {
			t.Errorf("Expected result map to contain the key 'id', but it wasn't found (result=%+v)", result)
		} else if _, ok := result["id"].(int64); !ok {
			t.Errorf("Expected result map to contain the key 'id' of type int64, but the cast failed (result=%T/%+v)", result, result)
		} else if id, _ := result["id"].(int64); id != int64(1) {
			t.Errorf("Expected result['id'] = int64(1), but instead actual=%T/%+v", id, id)
		}

		// Validate "name".
		if _, ok := result["name"]; !ok {
			t.Errorf("Expected result map to contain the key 'name', but it wasn't found (result=%+v)", result)
		}

		// Validate "home_planet".
		if _, ok := result["home_planet"]; !ok {
			t.Errorf("Expected result map to contain the key 'home_planet', but it wasn't found (result=%+v)", result)
		}
	}

	{
		var result map[string][]byte
		if err := driver.Raw(&result, `SELECT "name", "home_planet" FROM "my_datum" LIMIT 1`); err != nil {
			t.Fatal(err)
		}

		if l := len(result); l != 2 {
			t.Errorf("Expected result len=2, but actual result len=%v (result=%+v)", l, result)
		}

		// Validate "name".
		if name, ok := result["name"]; !ok {
			t.Errorf("Expected result map to contain the key 'name', but it wasn't found (result=%+v)", result)
		} else if string(name) != "Name0" {
			t.Errorf("Expected result['name'] = 'Name0', but instead actual bytes=%+v string='%v'", name, string(name))
		}

		// Validate "home_planet".
		if homePlanet, ok := result["home_planet"]; !ok {
			t.Errorf("Expected result map to contain the key 'home_planet', but it wasn't found (result=%+v)", result)
		} else if string(homePlanet) != "Zurg" {
			t.Errorf("Expected result['home_planet'] = 'Zurg', but instead actual bytes=%+v string='%v'", homePlanet, string(homePlanet))
		}
	}

	{
		var result []map[string]string
		if err := driver.Raw(&result, `SELECT "name" FROM "my_datum" ORDER BY "id" ASC`); err != nil {
			t.Fatal(err)
		}

		if l := len(result); l != n {
			t.Errorf("Expected result len=%v, but actual result len=%v", n, l)
		}
	}

	{
		var result []map[string]interface{}
		if err := driver.Raw(&result, `SELECT "name" FROM "my_datum" ORDER BY "id" ASC`); err != nil {
			t.Fatal(err)
		}

		if l := len(result); l != n {
			t.Errorf("Expected result len=%v, but actual result len=%v", n, l)
		}
	}
}

func TestTableName(t *testing.T) {
	driver, cleanupFunc := reset(t, dbDriverName, dbConnectionStrings)
	defer cleanupFunc()
	if tableName := driver.TableName(&MyDatum{}); tableName != "my_datum" {
		t.Errorf("Expected table name='my_datum' but actual='%v'", tableName)
	}
}
