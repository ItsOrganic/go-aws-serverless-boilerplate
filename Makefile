build:
	# Clean any previous builds
	rm -f bootstrap

	# One binary serves both functions (APP_MODE selects api / csv-processor).
	# Custom runtimes require the executable to be named "bootstrap".
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-ldflags="-s -w" \
		-tags lambda.norpc \
		-o bootstrap \
		./src

	chmod +x bootstrap

deploy: build
	serverless deploy --stage prod

run:
	go run ./src

migrate:
	psql "$(DATABASE_URL)" -f migrations/001_init.sql

clean:
	rm -f bootstrap function.zip
	rm -rf ./bin ./vendor .serverless

.PHONY: build deploy run migrate clean
