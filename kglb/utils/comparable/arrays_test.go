package comparable

import (
	. "gopkg.in/check.v1"

	. "godropbox/gocheck2"
)

type ArraysDiffSuite struct {
}

var _ = Suite(&ArraysDiffSuite{})

// Validate functionality on array of strings.
func (m *ArraysDiffSuite) TestBasicStrings(c *C) {

	// Comparable interface implementation.
	comparable := &ComparableImpl{
		KeyFunc: func(item interface{}) string {
			return item.(string)
		},
		EqualFunc: func(item1, item2 interface{}) bool {
			return item1.(string) == item2.(string)
		},
	}

	result := CompareArrays(
		[]interface{}{
			"a",
			"b",
			"c",
		},
		[]interface{}{
			"b",
			"d",
		},
		comparable)

	c.Assert(result.Added, DeepEqualsPretty, []interface{}{"d"})
	c.Assert(len(result.Deleted), Equals, 2) // deleted "a", "c"
	c.Assert(result.Unchanged, DeepEqualsPretty, []interface{}{"b"})
	c.Assert(result.Changed, IsNil)
}

// Validate functionality on array of strings.
func (m *ArraysDiffSuite) TestChanges(c *C) {
	type testItem struct {
		el1 string
		el2 int
	}

	// Comparable interface implementation.
	comparable := &ComparableImpl{
		KeyFunc: func(item interface{}) string {
			return item.(*testItem).el1
		},
		EqualFunc: func(item1, item2 interface{}) bool {
			return item1.(*testItem).el2 == item2.(*testItem).el2
		},
	}

	result := CompareArrays(
		[]interface{}{
			&testItem{el1: "1", el2: 10},
			&testItem{el1: "2", el2: 20},
			&testItem{el1: "3", el2: 30},
		},
		[]interface{}{
			&testItem{el1: "2", el2: 40}, // modify second
			&testItem{el1: "3", el2: 30},
			&testItem{el1: "4", el2: 10},
		},

		comparable)

	c.Assert(result.Added, DeepEqualsPretty, []interface{}{
		&testItem{el1: "4", el2: 10},
	})
	c.Assert(result.Deleted, DeepEqualsPretty, []interface{}{
		&testItem{el1: "1", el2: 10},
	})
	c.Assert(result.Unchanged, DeepEqualsPretty, []interface{}{
		&testItem{el1: "3", el2: 30},
	})
	c.Assert(result.Changed, DeepEqualsPretty, []ChangedPair{
		ChangedPair{
			OldItem: &testItem{el1: "2", el2: 20},
			NewItem: &testItem{el1: "2", el2: 40},
		},
	})
}
