package migrations

import _ "embed"

// Core is the QA-01 bootstrap migration; subsequent changes need new migrations.
//
//go:embed 001_core.sql
var Core string

//go:embed 002_devices.sql
var Devices string
