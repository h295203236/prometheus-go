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

	"github.com/BurntSushi/toml"
	promql "github.com/h295203236/prometheus/promql/parser"
)

// Config info
type Config struct {
	Pattern    string `toml:"pattern"`
	Port       int    `toml:"port"`
	PromServer string `toml:"prometheus_server"`
}

// HTTPForward prometheus http requests forward.
func HTTPForward(pattern string, port int) {
	fmt.Printf("listening on: :%v%v\n", port, pattern)
	http.HandleFunc(pattern, httpServe)
	err := http.ListenAndServe(":"+strconv.Itoa(port), nil)
	if err != nil {
		fmt.Println(err)
		os.Exit(100)
	}
}

func httpServe(w http.ResponseWriter, r *http.Request) {
	client := &http.Client{}
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Println(err)
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
		fmt.Println(err)
		return
	}
	// Set Request Header and Do Request
	for k, v := range r.Header {
		req.Header.Set(k, v[0])
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
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
		fmt.Println(err)
		return re
	}
	re = strings.ReplaceAll(parseExpr.String(), "__org_token__", token)
	fmt.Println(re)
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

var config *Config

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
	fmt.Println(filePath)
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
	initConf()
	fmt.Println(config)
	HTTPForward(config.Pattern, config.Port)
	// tmpData := `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"__name__":"node_memory_MemTotal_bytes","instance":"192.168.1.121","job":"Host","orgtoken":"1"},"values":[[1586416665,"3605553152"],[1586420265,"3605553152"]]}]}}`
	// tmpData := `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"orgtoken":"1"},"values":[[1586416665,"3605553152"],[1586420265,"3605553152"]]}]}}`
	// finalContents := removeTokenOfData(tmpData)
	// fmt.Println(finalContents)
}
