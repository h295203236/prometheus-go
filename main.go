// forward prometheus requests from grafana.
// token tag add impl deps prometheus/promql/parser/printer.go

package main

import (
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"log"

	"github.com/BurntSushi/toml"
	promql "github.com/h295203236/prometheus/promql/parser"
)

// Config info
type Config struct {
	Pattern     string `toml:"pattern"`
	Port        int    `toml:"port"`
	PromServer  string `toml:"prometheus_server"`
	EnableDebug bool   `toml:"debug"`
}

// HTTPForward prometheus http requests forward.
func HTTPForward(pattern string, port int) {
	log.Printf("[HTTPForward]listening on: :%d%s\n", port, pattern)
	http.HandleFunc(pattern, httpServe)
	err := http.ListenAndServe(":"+strconv.Itoa(port), nil)
	if err != nil {
		log.Printf("[HTTPForward]listening on: :%s\n", err)
		os.Exit(100)
	}
}

func httpServe(w http.ResponseWriter, r *http.Request) {
	client := &http.Client{}
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("[httpServe]read request body error: %s\n", err)
	}

	// Parse query and put the orgId labels.
	orgID := r.Header.Get("X-Grafana-Org-Id")
	query := r.URL.Query()
	if query.Get("query") != "" {
		queryParam := query.Get("query")
		queryParam = reGenerateQueryParam(queryParam, orgID)
		query.Set("query", queryParam)
	} else if query.Get("match[]") != "" {
		queryParam := query.Get("match[]")
		queryParam = reGenerateQueryParam(queryParam, orgID)
		query.Set("match[]", queryParam)
	}
	// Create request
	reqURL := fmt.Sprintf("%s/%s?%s", config.PromServer, r.URL.Path, query.Encode())
	req, err := http.NewRequest(r.Method, reqURL, strings.NewReader(string(body)))
	if err != nil {
		log.Printf("[httpServe]Create http.NewReuqest error: %s\n", err)
		return
	}
	// Set Request Header and Do Request
	for k, v := range r.Header {
		req.Header.Set(k, v[0])
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[httpServe]Create client.Do error: %s\n", err)
	}
	defer resp.Body.Close() // Close res when no need.

	// Boady data remove orgtoken info
	// Parser response data.
	bufRead := &bytes.Buffer{}
	contents, _ := ioutil.ReadAll(resp.Body)
	if resp.Header.Get("Content-Encoding") == "gzip" {
		resp.Header.Del("Content-Length")
		zr, _ := gzip.NewReader(bytes.NewReader(contents))
		bufRead.ReadFrom(zr)
		zr.Close()
	} else {
		bufRead.ReadFrom(bytes.NewReader(contents))
	}
	finalConents := removeTokenOfData(bufRead.String())

	// Re-GZip response data.
	bufWrite := &bytes.Buffer{}
	if resp.Header.Get("Content-Encoding") == "gzip" {
		//resp.Header.Del("Content-Length")
		zw := gzip.NewWriter(bufWrite)
		zw.Write([]byte(finalConents))
		zw.Close()
	} else {
		bufWrite.Write([]byte(finalConents))
	}

	// Set Response Header and Return the response data
	for k, v := range resp.Header {
		w.Header().Set(k, v[0])
	}
	io.Copy(w, bytes.NewReader(bufWrite.Bytes()))
	// io.Copy(w, resp.Body)
}

func reGenerateQueryParam(expr string, token string) string {
	var re string
	parseExpr, err := promql.ParseExpr(expr)
	if err != nil {
		log.Printf("[reGenerateQueryParam]parser prometheus expr error: %s\n", err)
		return re
	}
	re = strings.ReplaceAll(parseExpr.String(), "__org_token__", token)
	if config.EnableDebug {
		log.Printf("[reGenerateQueryParam]paser src expr: %s\n", expr)
		log.Printf("[reGenerateQueryParam]paser dst expr: %s\n", re)
	}
	return re
}

func removeTokenOfData(contents string) string {
	reg := regexp.MustCompile(`(,?\s?"orgtoken"\s?:\s?"\w+")`)
	newContents := reg.ReplaceAllString(contents, "")
	return newContents
}

// PathExists if the file path exist.
func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

var (
	config *Config
	logger *log.Logger
)

func initConf() {
	filePath := "./config.toml"
	flag.StringVar(&filePath, "conf", "./config.toml", "config file path")
	flag.Parse()
	getConf(filePath)
}
func getConf(filePath string) {
	if isExist, _ := PathExists(filePath); !isExist {
		panic("File path no exist: " + filePath)
	}
	if _, err := toml.DecodeFile(filePath, &config); err != nil {
		panic(err)
	}
	if config.Pattern == "" {
		config.Pattern = "/"
	}
	if config.Port == 0 {
		config.Port = 9090
	}
	if config.PromServer == "" {
		config.PromServer = "http://localhost:9090"
	}
}

func main() {
	logFile, err := os.OpenFile("server.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		panic("Failed to open log file: " + err.Error())
	}
	logger = log.New(logFile, "", log.Ldate|log.Ltime|log.LUTC|log.Llongfile)

	logger.Println("Starting init config...")
	initConf()
	logger.Printf("Starting server: :%d%s\n", config.Port, config.Pattern)
	HTTPForward(config.Pattern, config.Port)
	// tmpData := `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"__name__":"node_memory_MemTotal_bytes","instance":"192.168.1.121","job":"Host","orgtoken":"1"},"values":[[1586416665,"3605553152"],[1586420265,"3605553152"]]}]}}`
	// tmpData := `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"orgtoken":"1"},"values":[[1586416665,"3605553152"],[1586420265,"3605553152"]]}]}}`
	// finalContents := removeTokenOfData(tmpData)
	// fmt.Println(finalContents)
}
