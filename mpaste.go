package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/Mango0x45/getgopt"
	"github.com/alecthomas/chroma/formatters/html"
	"github.com/alecthomas/chroma/lexers"
	"github.com/alecthomas/chroma/styles"
	"github.com/dgrijalva/jwt-go"
)

const (
	urlHomepage = iota
	urlInvalid
	urlSyntax
	urlValid
)

var (
	counterFile string
	counter     int
	domain      string
	filePrefix  string
	indexFile   string
	mutex       sync.Mutex
	secretKey   = os.Getenv("MPASTE_SECRET")
	style       = styles.Get("monokai")
	userFile    string
)

func usage() {
	fmt.Fprintf(os.Stderr,
		"Usage: %s [-c file] [-f directory] [-i file] [-u file] domain port\n",
		os.Args[0])
	os.Exit(1)
}

func die(e interface{}) {
	fmt.Fprintln(os.Stderr, e)
	os.Exit(1)
}

func writeHeader(w http.ResponseWriter, h int, s string) {
	w.WriteHeader(h)
	if s == "" {
		fmt.Fprintln(w, http.StatusText(h))
	} else {
		fmt.Fprintln(w, s)
	}
}

func removeExt(s string) string {
	return strings.TrimSuffix(s, path.Ext(s))
}

func allowedUser(name string) bool {
	mutex.Lock()
	defer mutex.Unlock()

	if _, err := os.Stat(userFile); os.IsNotExist(err) {
		return false
	}

	file, err := os.Open(userFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return false
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		if scanner.Text() == name {
			return true
		}
	}

	return false
}

func validateToken(r *http.Request) bool {
	token, _ := jwt.Parse(r.Header.Get("Authorization"), func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, nil
		}
		return []byte(secretKey), nil
	})

	if token == nil {
		return false
	}

	claims, ok := token.Claims.(jwt.MapClaims)

	if !(ok && token.Valid) {
		return false
	}

	if userFile == "" {
		return true
	}

	return allowedUser(claims["name"].(string))
}

func isValidUrl(s string) int {
	var i int
	var c rune
	for i, c = range s {
		if c == '.' && i > 0 {
			return urlSyntax
		} else if c < '0' || c > '9' {
			return urlInvalid
		}
	}

	if c != 0 {
		return urlValid
	}
	return urlHomepage
}

func syntaxHighlighting(w http.ResponseWriter, r *http.Request) {
	lexer := lexers.Match(r.URL.Path[1:])
	if lexer == nil {
		http.ServeFile(w, r, filePrefix+r.URL.Path[1:])
		return
	}

	data, err := ioutil.ReadFile(filePrefix + removeExt(r.URL.Path[1:]))
	if err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		writeHeader(w, http.StatusNotFound, "")
		return
	}

	iterator, err := lexer.Tokenise(nil, string(data))
	if err != nil {
		writeHeader(w, http.StatusInternalServerError, "Failed to tokenize output")
		return
	}

	tw, err := strconv.Atoi(r.URL.Query().Get("tabs"))
	if err != nil {
		tw = 8
	}
	formatter := html.New(html.Standalone(true), html.WithClasses(true),
		html.WithLineNumbers(true), html.LineNumbersInTable(true), html.TabWidth(tw))
	if err := formatter.Format(w, style, iterator); err != nil {
		writeHeader(w, http.StatusInternalServerError, "Failed to format output")
		return
	}
}

func endpoint(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		switch isValidUrl(r.URL.Path[1:]) {
		case urlHomepage:
			http.ServeFile(w, r, indexFile)
		case urlInvalid:
			writeHeader(w, http.StatusNotFound, "")
			return
		case urlSyntax:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			syntaxHighlighting(w, r)
		case urlValid:
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			http.ServeFile(w, r, filePrefix+r.URL.Path[1:])
		}
	case http.MethodPost:
		if secretKey != "" && !validateToken(r) {
			writeHeader(w, http.StatusForbidden, "")
			return
		}

		file, _, err := r.FormFile("data")
		defer file.Close()
		if err != nil {
			writeHeader(w, http.StatusInternalServerError, "Failed to parse form")
			return
		}

		mutex.Lock()
		defer mutex.Unlock()

		fname := filePrefix + strconv.Itoa(counter)
		nfile, err := os.Create(fname)
		defer nfile.Close()
		if err != nil {
			writeHeader(w, http.StatusInternalServerError, "Failed to create file")
			return
		}

		if _, err = io.Copy(nfile, file); err != nil {
			writeHeader(w, http.StatusInternalServerError, "Failed to write file")
			return
		}

		if err = os.WriteFile(counterFile, []byte(strconv.Itoa(counter+1)), 0644); err != nil {
			writeHeader(w, http.StatusInternalServerError, "Failed to update counter")
			return
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, domain+"/%d\n", counter)

		counter++
	default:
		writeHeader(w, http.StatusMethodNotAllowed, "Only GET and POST requests are supported")
	}
}

func main() {
	for opt := byte(0); getgopt.Getopt(len(os.Args), os.Args, ":c:f:i:u:", &opt); {
		switch opt {
		case 'c':
			counterFile = getgopt.Optarg
		case 'f':
			filePrefix = getgopt.Optarg
		case 'i':
			indexFile = getgopt.Optarg
		case 'u':
			userFile = getgopt.Optarg
		default:
			usage()
		}
	}

	argv := os.Args[getgopt.Optind:]
	if len(argv) != 2 {
		usage()
	}
	domain = argv[0]
	port := argv[1]

	if filePrefix == "" {
		filePrefix = "files/"
	} else if filePrefix[len(filePrefix)-1] != '/' {
		filePrefix += "/"
	}

	if counterFile == "" {
		counterFile = "counter"
	}

	if indexFile == "" {
		indexFile = "index.html"
	}

	if _, err := os.Stat(indexFile); os.IsNotExist(err) {
		die(err)
	}

	if _, err := os.Stat(filePrefix); os.IsNotExist(err) {
		if err = os.MkdirAll(filePrefix, 0755); err != nil {
			die(err)
		}
	}

	if _, err := os.Stat(counterFile); os.IsNotExist(err) {
		counter = 0
	} else {
		data, err := ioutil.ReadFile(counterFile)
		if err != nil {
			die(err)
		}
		counter, _ = strconv.Atoi(string(data))
	}

	http.HandleFunc("/", endpoint)
	die(http.ListenAndServe(":"+port, nil))
}
