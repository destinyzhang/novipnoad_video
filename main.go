package main

import (
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

//播放信息
type playRs struct {
	name  string
	plist string
}

//multilink
type multilinkInfo struct {
	name    string
	playRss []*playRs
}

//当前页面信息
type pageInfo struct {
	title          string
	pkey           string
	multilinkInfos []*multilinkInfo
}

type quality struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Type string `json:"type"`
}

type videoJSON struct {
	Code           int       `json:"code"`
	QualityList    []quality `json:"quality"`
	DefaultQuality int       `json:"defaultQuality"`
}

func rc4(e string) string {
	tlen := len(rc4Code)
	j, _ := base64.StdEncoding.DecodeString(e)
	l := make(map[int]int)
	b, k, m := 0, 0, 0
	for ; 256 > m; m++ {
		l[m] = m
	}
	for m = 0; 256 > m; m++ {
		k = (k + l[m] + int(rune(rc4Code[m%tlen]))) % 256
		f := l[m]
		l[m] = l[k]
		l[k] = f
	}
	b, k, m = 0, 0, 0
	jlen := len(j)
	i := ""
	for ; b < jlen; b++ {
		m = (m + 1) % 256
		k = (k + l[m]) % 256
		f := l[m]
		l[m] = l[k]
		l[k] = f
		i += string(rune(j[b]) ^ rune(l[(l[m]+l[k])%256]))
	}
	return i
}

func e(c1 int, a int) string {
	var r1 string
	var r2 string
	if c1 < a {
		r1 = ""
	} else {
		r1 = e(c1/a, a)
	}
	c1 = c1 % a
	if c1 > 35 {
		r2 = string(rune(c1 + 29))
	} else {
		r2 = strconv.FormatInt(int64(c1), 36)
	}
	return r1 + r2
}

func packed(p string, a int, c int, k []string) map[string]string {
	d := make(map[string]string)
	length := len(k)
	for {
		if c <= 0 {
			break
		}
		c--
		key := e(c, a)
		if c >= length {
			d[key] = key
		} else {
			if len(k[c]) == 0 {
				d[key] = key
			} else {
				d[key] = k[c]
			}
		}

	}
	pat := `\b\w+\b`
	re, _ := regexp.Compile(pat)
	funcR := func(sss string) string {
		return d[sss]
	}
	str2 := strings.ReplaceAll(strings.ReplaceAll(re.ReplaceAllStringFunc(p, funcR), "+", ""), "\"", "")
	str3 := str2[strings.Index(str2, "{")+1 : strings.Index(str2, "}")]
	result := make(map[string]string)
	for _, v := range strings.Split(str3, ",") {
		idx := strings.Index(v, ":")
		key := v[0:idx]
		value := v[idx+1:]
		result[key] = value
	}
	return result
}

func getRequest(url string, header map[string]string) *http.Response {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Println("CreateURLRequest url:" + url + "error:" + err.Error())
		return nil
	}
	if header != nil {
		for k, v := range header {
			req.Header.Set(k, v)
		}
	}
	resp, err2 := (&http.Client{}).Do(req)
	if err2 != nil {
		fmt.Println("HTTPRequest url:" + url + "error:" + err2.Error())
		return nil
	}
	return resp
}

func doRequestDocument(url string, header map[string]string) *goquery.Document {
	resp := getRequest(url, header)
	if resp == nil {
		return nil
	}
	defer resp.Body.Close()
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil
	}
	return doc
}

func doRequestString(url string, header map[string]string) string {
	resp := getRequest(url, header)
	if resp == nil {
		return ""
	}
	defer resp.Body.Close()
	var body string
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		reader, _ := gzip.NewReader(resp.Body)
		for {
			buf := make([]byte, 1024)
			n, err := reader.Read(buf)

			if err != nil && err != io.EOF {
				panic(err)
			}

			if n == 0 {
				break
			}
			body += string(buf)
		}
	default:
		bytes, err3 := ioutil.ReadAll(resp.Body)
		if err3 != nil {
			fmt.Println("unit HTTPRequest  ReadAll url:" + url + "error:" + err3.Error())
			return ""
		}
		body = string(bytes)
	}
	return body
}

func pickParmResult(url string, referer string, host string) (parmResult map[string]string, playerURL string) {
	defer func() {
		if r := recover(); r != nil {
			parmResult, playerURL = nil, ""
		}
	}()
	body := doNormal(url, referer, host)
	if body == "" {
		parmResult, playerURL = nil, ""
		return
	}
	parmStr1, parmStr2 := `return p}(`, `.split('|'),0,{})`
	pathStr1, pathStr2 := `<iframe src="`, `" width="100%" height="100%"`
	str3 := body[strings.Index(body, parmStr1)+len(parmStr1)+1 : strings.Index(body, parmStr2)-1]
	str4 := body[strings.Index(body, pathStr1)+len(pathStr1) : strings.Index(body, pathStr2)]
	parms := strings.Split(str3, "'")
	p1 := parms[0]
	k := strings.Split(parms[2], "|")
	parms = strings.Split(parms[1], ",")
	a, _ := strconv.Atoi(parms[1])
	c, _ := strconv.Atoi(parms[2])
	parmResult, playerURL = packed(p1, a, c, k), str4
	return
}

func encodeURIComponent(str string) string {
	r := url.QueryEscape(str)
	r = strings.Replace(r, "+", "%20", -1)
	return r
}

func doNormal(url string, referer string, host string) string {
	//请求参数
	header := getBaseHeader(url, referer)
	header["Upgrade-Insecure-Requests"] = "1"
	header["Accept"] = "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,*/*;q=0.8"
	return doRequestString(url, header)
}

func pickVideoURL(url string, referer string, host string) string {
	//请求参数
	header := getBaseHeader(url, referer)
	header["Accept"] = "*/*"
	return doRequestString(url, header)
}

func getBaseHeader(host string, referer string) map[string]string {
	header := make(map[string]string)
	header["Host"] = host
	header["Referer"] = referer
	header["User-Agent"] = "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/70.0.3538.25 Safari/537.36 Core/1.70.3650.400 QQBrowser/10.4.3341.400"
	header["Accept-Encoding"] = "gzip, deflate"
	header["Accept-Language"] = "zh-CN,zh;q=0.9"
	header["Connection"] = "keep-alive"
	return header
}

var (
	help    bool
	path    string
	rc4Code string
)

const (
	host        = "http://www.novipnoad.com/"
	hostBonjour = "bonjour.sc2yun.com"
	hostAPI     = "api.upos.noanob.com"
)

func getURL() string {
	return host + path
}

func init() {
	flag.BoolVar(&help, "h", false, "帮助信息")
	flag.StringVar(&path, "p", "", "页面路径")
	flag.StringVar(&rc4Code, "c", "5c571074", "rc4密钥")
}

func usage() {
	fmt.Fprintf(os.Stderr, host+" 视频爬虫\nOptions:\n")
	flag.PrintDefaults()
}

//解析页面
func parsePage() *pageInfo {
	doc := doRequestDocument(getURL(), nil)
	if doc == nil {
		return nil
	}
	page := &pageInfo{multilinkInfos: make([]*multilinkInfo, 0, 5)}
	//解析title
	page.title = doc.Find("head").Find("title").Text()
	//取得主体
	articleBody := doc.Find("body").Find("div#body-wrap").Find("div#body").Find("article")
	//解析pkey
	pkeyScript := articleBody.Find("script")
	pkeyScript.Each(func(i int, s *goquery.Selection) {
		pkey := s.Text()
		pkey = pkey[strings.LastIndex(pkey, `$pkey="`)+len(`$pkey="`):]
		page.pkey = pkey[0:strings.Index(pkey, `"`)]
	})
	//解析linkhead linkhead保存了multilink的名字
	linkheads := articleBody.Find("p#linkhead")
	linkheads.Each(func(i int, linkhead *goquery.Selection) {
		multInfo := &multilinkInfo{name: linkhead.Text(), playRss: make([]*playRs, 0, 5)}
		page.multilinkInfos = append(page.multilinkInfos, multInfo)
	})

	findFunc := func(i int) string {
		linkhead := articleBody.Find(fmt.Sprintf("p#linkhead%d", i))
		return linkhead.First().Text()
	}
	//解析multilink
	multilinks := articleBody.Find("div.tm-multilink")
	multilinks.Each(func(i int, multilink *goquery.Selection) {
		var multInfo *multilinkInfo
		if i >= len(page.multilinkInfos) {
			multInfo = &multilinkInfo{name: findFunc(i), playRss: make([]*playRs, 0, 5)}
			page.multilinkInfos = append(page.multilinkInfos, multInfo)
		} else {
			multInfo = page.multilinkInfos[i]
		}
		multilink.Find("a").Each(func(j int, a *goquery.Selection) {
			if onclick, exist := a.Attr("onclick"); exist {
				rs := &playRs{name: a.Text()}
				rs.plist = onclick[strings.Index(onclick, `'`)+1 : strings.LastIndex(onclick, `'`)]
				multInfo.playRss = append(multInfo.playRss, rs)
			}
		})
	})
	return page
}

func pickMultilink(mutilink *multilinkInfo, pkey string) {
	if len(mutilink.playRss) == 0 {
		fmt.Printf("%s 未找到资源跳过\n", mutilink.name)
		return
	}
	fmt.Printf("%s\n", mutilink.name)
	for _, rs := range mutilink.playRss {
		pickRs(rs, pkey)
	}
}

func pickRs(rs *playRs, pkey string) {
	if rs.plist == "null" {
		fmt.Printf("%s %s 非法\n", rs.name, rs.plist)
		return
	}
	url := "http://" + hostBonjour + "/v1/?url=" + rs.plist + "&pkey=" + pkey + "&ref=/" + path
	result, _ := pickParmResult(url, getURL(), hostBonjour)
	if result == nil {
		fmt.Printf("pickParmResult %s 失败\n", rs.name)
		return
	}
	secondParm := "ckey=" + strings.ToUpper(result["ckey"]) + "&ref=" + encodeURIComponent(result["ref"]) + "&ip=" + result["ip"] + "&time=" + result["time"]
	playNames := strings.Split(rs.plist, `-`)
	body := pickVideoURL("http://"+hostAPI+"/"+playNames[0]+"/"+playNames[1]+".js?"+secondParm, url, hostAPI)
	if body == "" {
		fmt.Printf("pickVideoURL %s 失败\n", rs.name)
		return
	}
	vjson := videoJSON{}
	if strings.Contains(body, "JSON.decrypt") {
		rc4Str := body[strings.Index(body, `"`)+1 : strings.LastIndex(body, `"`)]
		videoURL := rc4(rc4Str)
		if videoURL == "" {
			fmt.Printf("rc4 %s %s 失败\n", rs.name, rc4Str)
			return
		}
		json.Unmarshal([]byte(videoURL), &vjson)
		if vjson.Code != 200 {
			fmt.Printf("vjson %s code %d 失败 %v %s\n", rs.name, vjson.Code, vjson, videoURL)
			return
		}
	} else {
		rc4Str := body[strings.Index(body, `=`)+1 : strings.LastIndex(body, `;`)]
		err := json.Unmarshal([]byte(rc4Str), &vjson)
		if err != nil {
			fmt.Printf("vjson %s code %d 失败 %v %s\n", rs.name, vjson.Code, vjson, rc4Str)
			return
		}
	}

	fmt.Printf("%s\n", rs.name)
	for _, q := range vjson.QualityList {
		fmt.Printf("%s\n", q.URL)
	}
}

func main() {
	flag.Parse()
	if help {
		usage()
		return
	}
	if len(path) == 0 {
		fmt.Println("-p参数输入页面路径")
		return
	}
	page := parsePage()
	if page == nil || len(page.multilinkInfos) == 0 {
		fmt.Printf("%s%s 未解析到视频", host, path)
		return
	}
	fmt.Printf("%s \n", page.title)
	for _, multilink := range page.multilinkInfos {
		pickMultilink(multilink, page.pkey)
	}
}
