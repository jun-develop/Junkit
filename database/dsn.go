// Package database provides driver-independent helpers shared by every database engine: error
// classification and DSN masking. Connections live in the engine packages (database/postgres).
package database

import "regexp"

// sensitiveRe matches "key=value" pairs whose key commonly holds a secret
// (password, token, api key, ...), for redacting connection strings in logs.
var sensitiveRe = regexp.MustCompile(
	`(?i)(\b(password|passwd|pwd|secret|token|api[_-]?key)\s*=\s*)(('[^']*')|("[^"]*")|(\S+))`,
)

// MaskDSN redacts secrets in a connection string with "********".
//
// Usage:
//
//	log.Info("connecting", zap.String("dsn", database.MaskDSN(dsn)))
func MaskDSN(dsn string) string {
	if dsn == "" {
		return dsn
	}

	return sensitiveRe.ReplaceAllString(dsn, `${1}********`)
}
