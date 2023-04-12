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

.PHONY: all install-deps package