LDFLAGS =
LDFLAGS_f2=-ldflags '-w -s $(LDFLAGS)'

all: build
build:
	CGO_ENABLED=0 go build -trimpath $(LDFLAGS_f2) -o servefiles
linux:
	GOOS=linux CGO_ENABLED=0 go build -trimpath $(LDFLAGS_f2) -o servefiles
	docker run --rm -v $(PWD):/working reeganexe/upx /working/servefiles
