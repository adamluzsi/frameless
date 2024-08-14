export TESTCASE_SEED=4941712044222485961

go test -race -run TestQueue/pubsubcontractsQueuegithubcom/adamluzsi/frameless/adapter/postgresql/internal/spechelperTestEntity/queue_when_a_subscription_is_made_and_messages_are_published_then_subscription_receives_the_messages

go test -race -v -run TestQueue/pubsubcontractsBlockinggithubcom/adamluzsi/frameless/adapter/postgresql  | grep -Fe 'frameless/adapter/postgresql'  | grep -vFe '=== RUN' | grep -vFe '--- PASS:' | grep -vFe '--- SKIP:' | grep -vF -e 'pubsubcontractsBlockinggithubcom/adamluzsi/frameless/adapter/postgresql/internal/spechelperTestEntity'

