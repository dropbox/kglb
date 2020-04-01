package exclog

import (
	"fmt"
	"time"
)

// Severity level for reported exceptions
type Severity int

const (
	Uncaught    Severity = 20
	Critical    Severity = 40
	Operational Severity = 50
	Noncritical Severity = 60
	UserError   Severity = 80
)

func Report(err error, severity Severity, id string) {
	fmt.Errorf("e: %v, severity: %v, id: %v", err, severity, id)
}

func PanicAndReport(err error) {
	panic(fmt.Sprintf("exclog: err: %s", err))
}
func PanicAndReportf(format string, args ...interface{}) {
	panic(fmt.Sprintf(format, args...))
}
func ReportAndCrash(err error) {
	panic(fmt.Sprintf("exclog: ReportAndCrash(): %v", err))
}
func Flush(wait time.Duration) {
}
