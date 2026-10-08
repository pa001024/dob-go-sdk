// 统一异常模型：HTTP 层与 GraphQL 层共用，调用方可按 Code 分支。
package dob

import "fmt"

// DobApiError 是 SDK 异常基类，携带机器可读的 Code。
type DobApiError struct {
	Message string
	Code    string
	Payload any
}

func (e *DobApiError) Error() string { return e.Message }

// DobHttpError 是 HTTP 状态异常（含连接失败、超时）。
type DobHttpError struct {
	DobApiError
	Status int
}

// DobGraphQLError 是 GraphQL errors 数组非空时抛出，Message 为首条错误信息。
type DobGraphQLError struct {
	DobApiError
	Errors []any
}

func httpErrorf(status int, code string, format string, args ...any) *DobHttpError {
	return &DobHttpError{DobApiError: DobApiError{Message: fmt.Sprintf(format, args...), Code: code}, Status: status}
}

func gqlErrorf(msg string, errs []any) *DobGraphQLError {
	return &DobGraphQLError{DobApiError: DobApiError{Message: msg, Code: "graphql_error", Payload: errs}, Errors: errs}
}
