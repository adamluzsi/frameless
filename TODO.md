- FINISH: /Users/adamluzsi/src/github.com/adamluzsi/frameless/pkg/cli/cli_test.go #Test_main

features:

- [ ] health.Monitor.Func -> HealthCheck func to use for populating initial health check report

flaky tests:
- [ ] cd adapter/postgresql ; go test -run TestNewLockerFactory/LockerFactory#01/Locker/Locker/Unlock/Unlocker/returned_value_behaves_like_a_locksLocker______when_context_is_a_lock_context_made_by_a_lock_call_and_context_is_cancelled_during_locking_then_it_will_return_back_with_the_context_error_while_also_unlocking_itself 
- [ ] TESTCASE_SEED=3340578929028692184 go test -run TestEventLogRepository_Options_CompressEventLog ./adapter/memory
- [ ] TESTCASE_SEED=6528372374173831731 go test -run TestQueue_combined -count 1024 -failfast ./adapter/memory
- [ ] TESTCASE_SEED=5753648921519236034 go test -run TestWithNoOverlap/The_task_can_execute_as_many_times_we_want ./pkg/tasker
- [ ] TESTCASE_SEED=3120239193720558802 go test -run TestCacheRepository/var_cr_memory.CacheRepository/Cache/refresh_RefreshQueryMany ./adapter/memory

features:
- [ ] pkg/docs - ctx based documentation writer, where a test can blurp out documentation and a mermaid sequence UML
- [ ] logger -> nullLoggingDetails should be ignored -> no warning 
  * more details needed on how to trigger this use-case

