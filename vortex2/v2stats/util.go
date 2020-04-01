package v2stats

import (
	"sort"
)

type byKey struct {
	keys   []string
	values []string
}

func (a byKey) Len() int {
	return len(a.keys)
}
func (a byKey) Swap(i, j int) {
	a.keys[i], a.keys[j] = a.keys[j], a.keys[i]
	a.values[i], a.values[j] = a.values[j], a.values[i]
}
func (a byKey) Less(i, j int) bool {
	return a.keys[i] < a.keys[j]
}

// Metric tags are key=value pairs.
type KV map[string]string

func (kv KV) WithTag(tagName, tagValue string) KV {
	kvCopy := KV{}
	for k, v := range kv {
		kvCopy[k] = v
	}
	kvCopy[tagName] = tagValue
	return kvCopy
}

// Convert all colection of key and values into string with {keyN=valN,...} format.
func (kv KV) String() string {
	keys := make([]string, 0, len(kv))
	values := make([]string, 0, len(kv))
	for key, value := range kv {
		keys = append(keys, key)
		values = append(values, value)
	}

	sort.Sort(byKey{keys: keys, values: values})
	var res string
	for i, key := range keys {
		res = res + key + "=" + values[i]
		if i != len(keys)-1 {
			res = res + ","
		}
	}

	return res
}
