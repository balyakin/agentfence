package errorsx

import "net/http"

func HTTPStatus(code string) int {
	switch code {
	case CodeUnauthorized:
		return http.StatusUnauthorized
	case CodeValidation, CodeConfigInvalid, CodeConfigExists, CodeDaemonNonLoopbackForbidden:
		return http.StatusBadRequest
	case CodeNotFound, CodeRunNotFound, CodeNotGitRepo:
		return http.StatusNotFound
	case CodeRepoDirty, CodeApplyConflict, CodeActiveRunExists, CodeScanBlocked, CodePostScanBlocked:
		return http.StatusConflict
	case CodeTimeout:
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}
