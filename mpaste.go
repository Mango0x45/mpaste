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
	URL_HOMEPAGE = iota
	URL_INVALID
	URL_SYNTAX
	URL_VALID
)

var (
	counter      int
	counter_file string
	domain       string
	file_prefix  string
	index_file   string
	mutex        sync.Mutex
	secret_key   = os.Getenv("MPASTE_SECRET")
	user_file    string
)

var (
	style     = styles.Get("pygments")
	formatter = html.New(html.Standalone(true), html.WithClasses(true),
		html.WithLineNumbers(true), html.LineNumbersInTable(true))
)

func usage() {
	fmt.Fprintf(os.Stderr,
		"Usage: %s [-c file] [-f directory] [-i file] [-u file] domain port\n",
		os.Args[0])
	os.Exit(1)
}

func error_and_die(e interface{}) {
	fmt.Fprintln(os.Stderr, e)
	os.Exit(1)
}

func remove_ext(s string) string {
	return strings.TrimSuffix(s, path.Ext(s))
}

func allowed_user(name string) bool {
	mutex.Lock()
	defer mutex.Unlock()

	if _, err := os.Stat(user_file); os.IsNotExist(err) {
		return false
	}

	file, err := os.Open(user_file)
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

func validate_token(r *http.Request) bool {
	token, _ := jwt.Parse(r.Header.Get("Authorization"), func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("Something went wrong\n")
		}
		return []byte(secret_key), nil
	})

	if token == nil {
		return false
	}

	claims, ok := token.Claims.(jwt.MapClaims)

	if !(ok && token.Valid) {
		return false
	}

	if user_file == "" {
		return true
	}

	return allowed_user(claims["name"].(string))
}

func is_valid_url(s string) int {
	var i int
	var c rune
	for i, c = range s {
		if c == '.' && i > 0 {
			return URL_SYNTAX
		} else if c < '0' || c > '9' {
			return URL_INVALID
		}
	}

	if c != 0 {
		return URL_VALID
	}
	return URL_HOMEPAGE
}

func syntax_highlighting(w http.ResponseWriter, r *http.Request) {
	lexer := lexers.Match(r.URL.Path[1:])
	if lexer == nil {
		http.ServeFile(w, r, file_prefix+r.URL.Path[1:])
		return
	}

	data, err := ioutil.ReadFile(file_prefix + remove_ext(r.URL.Path[1:]))
	if err != nil {
		WRITE_HEADER(http.StatusNotFound, "404 page not found")
	}

	iterator, err := lexer.Tokenise(nil, string(data))
	if err != nil {
		WRITE_HEADER(http.StatusInternalServerError, "Failed to tokenize output")
	}

	if err := formatter.Format(w, style, iterator); err != nil {
		WRITE_HEADER(http.StatusInternalServerError, "Failed to format output")
	}
}

func endpoint(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		switch is_valid_url(r.URL.Path[1:]) {
		case URL_HOMEPAGE:
			http.ServeFile(w, r, index_file)
		case URL_INVALID:
			WRITE_HEADER(http.StatusNotFound, "404 page not found")
		case URL_SYNTAX:
			w.Header().Set("Content-Type", "text/html")
			syntax_highlighting(w, r)
		case URL_VALID:
			w.Header().Set("Content-Type", "text/plain")
			http.ServeFile(w, r, file_prefix+r.URL.Path[1:])
		}
	case http.MethodPost:
		if secret_key != "" && !validate_token(r) {
			WRITE_HEADER(http.StatusForbidden, "Invalid API key")
		}

		file, _, err := r.FormFile("data")
		defer file.Close()
		if err != nil {
			WRITE_HEADER(http.StatusInternalServerError, "Failed to parse form")
		}

		mutex.Lock()
		defer mutex.Unlock()

		fname := file_prefix + strconv.Itoa(counter)
		nfile, err := os.Create(fname)
		defer nfile.Close()
		if err != nil {
			WRITE_HEADER(http.StatusInternalServerError, "Failed to create file")
		}

		if _, err = io.Copy(nfile, file); err != nil {
			WRITE_HEADER(http.StatusInternalServerError, "Failed to write file")
		}

		if err = os.WriteFile(counter_file, []byte(strconv.Itoa(counter+1)), 0644); err != nil {
			WRITE_HEADER(http.StatusInternalServerError, "Failed to update counter")
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, domain+"/%d\n", counter)

		counter++
	default:
		WRITE_HEADER(http.StatusMethodNotAllowed, "Only GET and POST requests are supported")
	}
}

func main() {
	for opt := byte(0); getgopt.Getopt(len(os.Args), os.Args, ":c:f:i:u:", &opt); {
		switch opt {
		case 'c':
			counter_file = getgopt.Optarg
		case 'f':
			file_prefix = getgopt.Optarg
		case 'i':
			index_file = getgopt.Optarg
		case 'u':
			user_file = getgopt.Optarg
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

	if file_prefix == "" {
		file_prefix = "files/"
	} else if file_prefix[len(file_prefix)-1] != '/' {
		file_prefix += "/"
	}

	if counter_file == "" {
		counter_file = "counter"
	}

	if index_file == "" {
		index_file = "index.html"
	}

	if _, err := os.Stat(index_file); os.IsNotExist(err) {
		error_and_die(err)
	}

	if _, err := os.Stat(file_prefix); os.IsNotExist(err) {
		if err = os.MkdirAll(file_prefix, 0755); err != nil {
			error_and_die(err)
		}
	}

	if _, err := os.Stat(counter_file); os.IsNotExist(err) {
		counter = 0
	} else {
		data, err := ioutil.ReadFile(counter_file)
		if err != nil {
			error_and_die(err)
		}
		counter, _ = strconv.Atoi(string(data))
	}

	http.HandleFunc("/", endpoint)
	error_and_die(http.ListenAndServe(":"+port, nil))
}
