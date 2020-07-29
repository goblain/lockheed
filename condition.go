package lockheed

import (
	"fmt"
)

type Condition struct {
	Operation  Operation
	Conditions *[]Condition
	Field      Field
	Value      interface{}
}

type Operation string

const (
	OperationContains Operation = "contains"
	OperationEquals   Operation = "equals"
	OperationAnd      Operation = "and"
	OperationOr       Operation = "or"
)

type Field string

const (
	FieldAcquired Field = "acquired"
	FieldTags     Field = "tags"
)

func (l *Lock) EvaluateSubconditions(c *Condition) (bool, error) {
	var results []bool
	for _, cond := range *c.Conditions {
		result, err := l.Evaluate(&cond)
		if err != nil {
			return false, err
		}
		results = append(results, result)
	}
	switch c.Operation {
	case OperationAnd:
		for _, result := range results {
			if result == false {
				return false, nil
			}
		}
		return true, nil
	case OperationOr:
		for _, result := range results {
			if result == true {
				return true, nil
			}
		}
		return false, nil
	}
	return false, fmt.Errorf("Unsupported condition operation %s", c.Operation)
}

func (l *Lock) Evaluate(c *Condition) (bool, error) {
	if c.Conditions != nil {
		return l.EvaluateSubconditions(c)
	} else {
		switch c.Field {
		case FieldTags:
			switch c.Operation {
			case OperationContains:
				for _, tag := range l.Tags {
					if tag == c.Value.(string) {
						return true, nil
					}
				}
				return false, nil
			}
		case FieldAcquired:
			switch c.Operation {
			case OperationEquals:
				acquired := false
				for _, lease := range l.Leases {
					if !lease.Expired() {
						acquired = true
					}
				}
				if acquired == c.Value.(bool) {
					return true, nil
				}
				return false, nil
			}
		}
		return false, fmt.Errorf("Unsupported field operation %s or field %s", c.Operation, c.Field)
	}
}
