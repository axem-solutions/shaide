package config

import "reflect"

type Policy struct {
	Required bool
	Secret   bool
	When     Condition
}

func (p Policy) Active(values Values) bool {
	if p.When == nil {
		return true
	}

	return p.When.Match(values)
}

type Condition interface {
	Match(Values) bool
	Dependencies() []Key
}

type equalsCondition struct {
	key      Key
	expected any
}

func WhenEquals(key Key, expected any) Condition {
	return equalsCondition{
		key:      key,
		expected: expected,
	}
}

func (c equalsCondition) Match(values Values) bool {
	actual, ok := values[c.key]
	return ok && reflect.DeepEqual(actual, c.expected)
}

func (c equalsCondition) Dependencies() []Key {
	return []Key{c.key}
}

type Values map[Key]any
