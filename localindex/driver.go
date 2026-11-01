// Package localindex stores the explorer's durable SQLite search index.
// It is independent of the collector's CGO-backed SQLite driver.
package localindex

import _ "modernc.org/sqlite"
