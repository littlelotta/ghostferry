package test

import (
	"fmt"
	"testing"
	"time"

	"github.com/Shopify/ghostferry"
	"github.com/Shopify/ghostferry/testhelpers"
)

func setupSingleEntryTable(f *testhelpers.TestFerry) {
	testhelpers.SeedInitialData(f.SourceDB, "gftest", "table1", 1)
	testhelpers.SeedInitialData(f.TargetDB, "gftest", "table1", 0)
}

func TestSelectUpdateBinlogCopy(t *testing.T) {
	testcase := testhelpers.IntegrationTestCase{
		T:           t,
		SetupAction: setupSingleEntryTable,
		Ferry:       testhelpers.NewTestFerry(),
	}

	testcase.Ferry.BeforeBatchCopyListener = func(batch *ghostferry.RowBatch) error {
		queries := make([]string, len(batch.Values()))
		for i, row := range batch.Values() {
			id := row[0].(int64)
			queries[i] = "UPDATE gftest.table1 SET data = 'changed' WHERE id = ?"

			go func(query string) {
				_, err := testcase.Ferry.SourceDB.Exec(query, id)
				if err != nil {
					panic(err)
				}
			}(queries[i])
		}

		// Waiting for sure until we can see the queries as they will be
		// locked due to the SELECT FOR UPDATE that is being performed.
		for !testhelpers.ProcessListContainsQueries(testcase.Ferry.SourceDB, queries) {
			time.Sleep(200 * time.Millisecond)
		}

		return nil
	}

	testcase.CustomVerifyAction = func(f *testhelpers.TestFerry) {
		testcase.AssertOnlyDataOnSourceAndTargetIs("changed")
	}

	testcase.Run()
}

func TestUpdateBinlogSelectCopy(t *testing.T) {
	testcase := &testhelpers.IntegrationTestCase{
		T:           t,
		SetupAction: setupSingleEntryTable,
		Ferry:       testhelpers.NewTestFerry(),
	}

	testcase.AfterStartBinlogStreaming = func(f *testhelpers.TestFerry) {
		_, err := f.SourceDB.Exec("UPDATE gftest.table1 SET data = 'changed' LIMIT 1")
		testhelpers.PanicIfError(err)
	}

	testcase.CustomVerifyAction = func(f *testhelpers.TestFerry) {
		testcase.AssertOnlyDataOnSourceAndTargetIs("changed")
	}

	testcase.Run()
}

func TestSelectCopyUpdateBinlog(t *testing.T) {
	testcase := &testhelpers.IntegrationTestCase{
		T:           t,
		SetupAction: setupSingleEntryTable,
		Ferry:       testhelpers.NewTestFerry(),
	}

	testcase.BeforeStoppingBinlogStreaming = func(f *testhelpers.TestFerry) {
		_, err := f.SourceDB.Exec("UPDATE gftest.table1 SET data = 'changed' LIMIT 1")
		testhelpers.PanicIfError(err)
	}

	testcase.CustomVerifyAction = func(f *testhelpers.TestFerry) {
		testcase.AssertOnlyDataOnSourceAndTargetIs("changed")
	}

	testcase.Run()
}

func TestOnlyDeleteRowWithMaxPrimaryKey(t *testing.T) {
	testcase := &testhelpers.IntegrationTestCase{
		T: t,
		SetupAction: func(f *testhelpers.TestFerry) {
			testhelpers.SeedInitialData(f.SourceDB, "gftest", "table1", 2)
			testhelpers.SeedInitialData(f.TargetDB, "gftest", "table1", 0)
		},
		Ferry: testhelpers.NewTestFerry(),
	}

	testcase.Ferry.DataIterationBatchSize = 1

	lastRowDeleted := false
	testcase.Ferry.BeforeBatchCopyListener = func(batch *ghostferry.RowBatch) error {
		if lastRowDeleted {
			return nil
		}

		r, err := testcase.Ferry.SourceDB.Exec("DELETE FROM gftest.table1 ORDER BY id DESC LIMIT 1")
		if err != nil {
			return err
		}

		rowsAffected, err := r.RowsAffected()
		if err != nil {
			return err
		}

		if rowsAffected != 1 {
			return fmt.Errorf("there should be 1 row deleted, but %d rows were deleted", rowsAffected)
		}

		lastRowDeleted = true

		return nil
	}

	testcase.Run()
}
