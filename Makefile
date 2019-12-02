flags=.makeFlags
VPATH=$(flags)
$(shell mkdir -p $(flags))

all: build

clean:
	rm -rf ./.makeFlags
		
build:
	docker build -t hashcloak/meson-client -f Dockerfile .
	@touch $(flags)/$@

send-txn: build
	docker run --mount type=bind,source=`pwd`/client.toml,target=/client.toml \
		--network nonvoting_testnet_nonvoting_test_net \
		hashcloak/meson-client \
		/client \
		-pk $(shell cat ./pk) \
		-c client.toml \
		-t gor \
		-s gor \
		-chain 5 
