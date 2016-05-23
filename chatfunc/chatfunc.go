package chatfunc

import (
	"fmt"
	"regexp"
)

type Data struct {
	Trigger  string
	Endpoint string

	// generated
	triggerExpr *regexp.Regexp
}

func (d *Data) Match(msg string) (bool, error) {
	if d.triggerExpr == nil {
		re, err := regexp.Compile(d.Trigger)
		if err != nil {
			return false, fmt.Errorf("compiling Trigger: %v", err)
		}
		d.triggerExpr = re
	}
	return d.triggerExpr.MatchString(msg), nil
}
