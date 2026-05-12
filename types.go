package ioa

type Logger interface {
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Importantf(format string, args ...interface{})
}

type nopLogger struct{}

func NopLogger() Logger                                       { return nopLogger{} }
func (nopLogger) Debugf(string, ...interface{})               {}
func (nopLogger) Infof(string, ...interface{})                {}
func (nopLogger) Warnf(string, ...interface{})                {}
func (nopLogger) Errorf(string, ...interface{})               {}
func (nopLogger) Importantf(string, ...interface{})           {}

type ToolDefinition struct {
	Type     string             `json:"type"`
	Function FunctionDefinition `json:"function"`
}

type FunctionDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}
