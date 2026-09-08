package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

func (c Config[T]) Resolve(p Prompter) (auto.ConfigMap, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}

	result := auto.ConfigMap{}
	values := Values{}

	for _, entry := range c.Entries {
		if !entry.Policy.Active(values) {
			continue
		}

		value, ok, err := entry.resolve(p)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", c.Key(entry.Key), err)
		}

		if entry.Policy.Required && (!ok || isEmpty(value)) {
			return nil, fmt.Errorf("%s is required", c.Key(entry.Key))
		}

		if !ok {
			continue
		}

		encoded, err := encode(value)
		if err != nil {
			return nil, fmt.Errorf("encode %s: %w", c.Key(entry.Key), err)
		}

		values[entry.Key] = value

		result[c.Key(entry.Key)] = auto.ConfigValue{
			Value:  encoded,
			Secret: entry.Policy.Secret,
		}
	}

	return result, nil
}

func encode(value any) (string, error) {
	switch value := value.(type) {
	case string:
		return value, nil
	case bool:
		return strconv.FormatBool(value), nil
	case int:
		return strconv.Itoa(value), nil
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
}

func isEmpty(value any) bool {
	if value == nil {
		return true
	}

	switch value := value.(type) {
	case string:
		return strings.TrimSpace(value) == ""
	}

	v := reflect.ValueOf(value)

	switch v.Kind() {
	case reflect.Slice, reflect.Map:
		return v.Len() == 0
	}

	return false
}
