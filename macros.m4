define(`m4_len', defn(`len'))
undefine(`len')
define(`WRITEHEADER',
       `w.WriteHeader($1); ifelse(eval(m4_len($2) == 0), 0, fmt.Fprintln(w, $2);) return;')
