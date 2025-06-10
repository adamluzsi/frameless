package queries

import "fmt"

const CreateTableSchemaMigrationsTmpl = `CREATE TABLE IF NOT EXISTS %s (
	namespace  TEXT    NOT NULL,
	version    TEXT    NOT NULL,
	dirty      BOOLEAN NOT NULL
)`

const DropTableTmpl = `DROP TABLE IF EXISTS %s`

const CreateTableCacheEntitiesTmpl = `CREATE TABLE %s (
    id VARCHAR(255) PRIMARY KEY,
    data JSON NOT NULL
)`

const CreateTableCacheHitsTmpl = `CREATE TABLE %s (
    query_id  VARCHAR(255) PRIMARY KEY,
    ent_ids   JSON NOT NULL,
    timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL
)`

const CreateTableLocker = `
CREATE TABLE IF NOT EXISTS frameless_guard_locks (
    name VARCHAR(255) PRIMARY KEY
)`

const CreateTableTaskerScheduleStates = `
CREATE TABLE IF NOT EXISTS frameless_tasker_schedule_states ( 
    id        VARCHAR(255) PRIMARY KEY,
    timestamp TIMESTAMP NOT NULL
)`

var DropTableTaskerScheduleStates = fmt.Sprintf(DropTableTmpl, "frameless_tasker_schedule_states")
