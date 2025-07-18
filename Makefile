build:
	# Clean any previous builds
	rm -f bootstrap function.zip
	
	# Build with proper flags for Lambda
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-ldflags="-s -w" \
		-a -installsuffix cgo \
		-o bootstrap \
		src/main.go
	
	# Make sure bootstrap is executable
	chmod +x bootstrap
	
	# Create zip file
	zip function.zip bootstrap

deploy: build
	serverless deploy --stage prod

clean:
	rm -f bootstrap function.zip
	rm -rf ./bin ./vendor Gopkg.lock ./serverless

.PHONY: build deploy clean