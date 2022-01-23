.POSIX:

MANDIR	= /usr/share/man/man1
target	= mpaste

all: ${target}
${target}: mpaste.go
	go build

docs:
	>/dev/null command -v gzip && gzip -c9 mpaste.1 >${MANDIR}/mpaste.1.gz || \
		cp mpaste.1 ${MANDIR}

clean:
	rm -rf ${target} counter files/
.PHONY: clean
