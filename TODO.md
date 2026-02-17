# TODO

- FINISH: /Users/adamluzsi/src/github.com/adamluzsi/frameless/pkg/cli/cli_test.go #Test_main

features:

- [ ] health.Monitor.Func -> HealthCheck func to use for populating initial health check report

flaky tests:

- [ ] TESTCASE_SEED=3340578929028692184 go test -run TestEventLogRepository_Options_CompressEventLog ./adapter/memory
- [ ] TESTCASE_SEED=6528372374173831731 go test -run TestQueue_combined -count 1024 -failfast ./adapter/memory
- [ ] TESTCASE_SEED=5753648921519236034 go test -run TestWithNoOverlap/The_task_can_execute_as_many_times_we_want ./pkg/tasker
- [ ] TESTCASE_SEED=3120239193720558802 go test -run TestCacheRepository/var_cr_memory.CacheRepository/Cache/refresh_RefreshQueryMany ./adapter/memory

features:

- [ ] pkg/docs - ctx based documentation writer, where a test can blurp out documentation and a mermaid sequence UML
- [ ] logger -> nullLoggingDetails should be ignored -> no warning
  - more details needed on how to trigger this use-case
