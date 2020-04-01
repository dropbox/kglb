package comparable

// Construct key of the abstract element.
type Keyable interface {
	// Construct key for the the item.
	Key(item interface{}) string
}

// Set of required function to perform comparison on top of array of abstract
// elements.
type Comparable interface {
	Keyable
	// Compare two items and returns true when they are equal, otherwise false.
	Equal(item1, item2 interface{}) bool
}

// implementation of Comparable interface with required func as callbacks to
// simplify constructing Comparable impls on a fly.
type ComparableImpl struct {
	KeyFunc   func(item interface{}) string
	EqualFunc func(item1, item2 interface{}) bool
}

func (c *ComparableImpl) Key(item interface{}) string {
	return c.KeyFunc(item)
}

func (c *ComparableImpl) Equal(item1, item2 interface{}) bool {
	return c.EqualFunc(item1, item2)
}

// Pair of states of the item in the old and new states.
type ChangedPair struct {
	OldItem interface{}
	NewItem interface{}
}

// Result of CompareArrays with explicit field naming instead of using multiple
// return arguments which may be accidentally misplaces/misordered.
type ComparableResult struct {
	Added     []interface{}
	Deleted   []interface{}
	Changed   []ChangedPair
	Unchanged []interface{}
}

// Returns true when there is any change, otherwise false.
func (c *ComparableResult) IsChanged() bool {
	return len(c.Added) != 0 || len(c.Deleted) != 0 || len(c.Changed) != 0
}

// return new state of changed elements.
func (c *ComparableResult) NewChangedStates() []interface{} {
	result := make([]interface{}, len(c.Changed))
	for i, val := range c.Changed {
		result[i] = val.NewItem
	}
	return result
}

// Compare to arrays of elements and returns multiple arrays with added, deleted,
// changed and unmodified elements.
func CompareArrays(
	oldSet,
	newSet []interface{},
	comparable Comparable) *ComparableResult {

	result := &ComparableResult{}
	oldMap := keyGen(oldSet, comparable)
	newMap := keyGen(newSet, comparable)

	// 1. identify deleted, updated and unchanged elements.
	for key, oldItem := range oldMap {
		if newItem, ok := newMap[key]; !ok {
			result.Deleted = append(result.Deleted, oldItem)
		} else {
			if !comparable.Equal(oldItem, newItem) {
				result.Changed = append(result.Changed, ChangedPair{
					OldItem: oldItem,
					NewItem: newItem,
				})
			} else {
				result.Unchanged = append(result.Unchanged, oldItem)
			}
		}
	}

	// 2. identify new items.
	for key, newItem := range newMap {
		if _, ok := oldMap[key]; !ok {
			result.Added = append(result.Added, newItem)
		}
	}

	return result
}

// generate key for each item from array and return key -> item map.
func keyGen(items []interface{}, keyable Keyable) map[string]interface{} {
	keyMap := make(map[string]interface{}, len(items))
	for _, item := range items {
		key := keyable.Key(item)
		keyMap[key] = item
	}

	return keyMap
}
