.POSIX:

MANDIR	= /usr/share/man/man1
target	= mpaste

all: ${target}
${target}: macros.m4 mpaste.go
	m4 macros.m4 mpaste.go >tmp.go
	go build tmp.go
	mv tmp ${target}
	rm tmp.go

docs:
	>/dev/null command -v gzip && gzip -c9 mpaste.1 >${MANDIR}/mpaste.1.gz || \
		cp mpaste.1 ${MANDIR}

clean:
	rm -rf ${target} tmp.go counter files/
.PHONY: clean
