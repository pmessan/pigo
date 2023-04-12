all: install-deps package

clean:
	@rm -f pigo
install: all
	@cp pigo /usr/local/bin
uninstall:
	@rm -f /usr/local/bin/pigo
package:
	./build.sh package
install-deps: 
	go get -d ./...
	go install golang.org/x/tools/cmd/goimports@latest 

.PHONY: all install-deps package