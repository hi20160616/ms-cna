package fetcher

import (
	"crypto/md5"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/hi20160616/exhtml"
	"github.com/hi20160616/gears"
	"github.com/hi20160616/ms-cna/configs"
	"github.com/hycka/gocc"
	"github.com/pkg/errors"
	"golang.org/x/net/html"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Article struct {
	Id            string
	Title         string
	Content       string
	WebsiteId     string
	WebsiteDomain string
	WebsiteTitle  string
	UpdateTime    *timestamppb.Timestamp
	U             *url.URL
	raw           []byte
	doc           *html.Node
}

func NewArticle() *Article {
	return &Article{
		WebsiteDomain: configs.Data.MS["cna"].Domain,
		WebsiteTitle:  configs.Data.MS["cna"].Title,
		WebsiteId:     fmt.Sprintf("%x", md5.Sum([]byte(configs.Data.MS["cna"].Domain))),
	}
}

// List get all articles from database
func (a *Article) List() ([]*Article, error) {
	return load()
}

// Get read database and return the data by rawurl.
func (a *Article) Get(id string) (*Article, error) {
	as, err := load()
	if err != nil {
		return nil, err
	}

	for _, a := range as {
		if a.Id == id {
			return a, nil
		}
	}
	return nil, fmt.Errorf("[%s] no article with id: %s",
		configs.Data.MS["cna"].Title, id)
}

func (a *Article) Search(keyword ...string) ([]*Article, error) {
	as, err := load()
	if err != nil {
		return nil, err
	}

	as2 := []*Article{}
	for _, a := range as {
		for _, v := range keyword {
			v = strings.ToLower(strings.TrimSpace(v))
			switch {
			case a.Id == v:
				as2 = append(as2, a)
			case a.WebsiteId == v:
				as2 = append(as2, a)
			case strings.Contains(strings.ToLower(a.Title), v):
				as2 = append(as2, a)
			case strings.Contains(strings.ToLower(a.Content), v):
				as2 = append(as2, a)
			case strings.Contains(strings.ToLower(a.WebsiteDomain), v):
				as2 = append(as2, a)
			case strings.Contains(strings.ToLower(a.WebsiteTitle), v):
				as2 = append(as2, a)
			}
		}
	}
	return as2, nil
}

type ByUpdateTime []*Article

func (u ByUpdateTime) Len() int      { return len(u) }
func (u ByUpdateTime) Swap(i, j int) { u[i], u[j] = u[j], u[i] }
func (u ByUpdateTime) Less(i, j int) bool {
	return u[i].UpdateTime.AsTime().Before(u[j].UpdateTime.AsTime())
}

var timeout = func() time.Duration {
	t, err := time.ParseDuration(configs.Data.MS["cna"].Timeout)
	if err != nil {
		log.Printf("[%s] timeout init error: %v", configs.Data.MS["cna"].Title, err)
		return time.Duration(1 * time.Minute)
	}
	return t
}()

// fetchArticle fetch article by rawurl
func (a *Article) fetchArticle(rawurl string) (*Article, error) {
	translate := func(x string, err error) (string, error) {
		if err != nil {
			return "", err
		}
		tw2s, err := gocc.New("tw2s")
		if err != nil {
			return "", err
		}
		return tw2s.Convert(x)
	}

	var err error
	a.U, err = url.Parse(rawurl)
	if err != nil {
		return nil, err
	}
	// Dail
	a.raw, a.doc, err = exhtml.GetRawAndDoc(a.U, timeout)
	if err != nil {
		return nil, err
	}

	a.Id = fmt.Sprintf("%x", md5.Sum([]byte(rawurl)))

	a.Title, err = translate(a.fetchTitle())
	if err != nil {
		return nil, err
	}

	a.UpdateTime, err = a.fetchUpdateTime()
	if err != nil {
		return nil, err
	}

	// content should be the last step to fetch
	a.Content, err = translate(a.fetchContent())
	if err != nil {
		return nil, err
	}

	a.Content, err = a.fmtContent(a.Content)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (a *Article) fetchTitle() (string, error) {
	n := exhtml.ElementsByTag(a.doc, "title")
	if n == nil {
		return "", fmt.Errorf("[%s] getTitle error, there is no element <title>", configs.Data.MS["cna"].Title)
	}
	title := n[0].FirstChild.Data
	if strings.Contains(title, "| ?????? |") ||
		strings.Contains(title, "| ?????? |") ||
		strings.Contains(title, "| ?????? |") ||
		strings.Contains(title, "| ?????? |") ||
		strings.Contains(title, "| ?????? |") ||
		strings.Contains(title, "| ?????? |") ||
		strings.Contains(title, "| ?????? |") ||
		strings.Contains(title, "| ?????? |") ||
		strings.Contains(title, "| ?????? |") ||
		strings.Contains(title, "| ?????? |") ||
		strings.Contains(title, "| ?????? |") {
		return "", fmt.Errorf("[%s] ignore post on purpose: %s",
			configs.Data.MS["cna"].Title, a.U.String())
	}
	title = strings.ReplaceAll(title, " | ????????? CNA", "")
	title = strings.TrimSpace(title)
	gears.ReplaceIllegalChar(&title)
	return title, nil
}

func (a *Article) fetchUpdateTime() (*timestamppb.Timestamp, error) {
	if a.raw == nil {
		return nil, errors.Errorf("[%s] fetchUpdateTime: raw is nil: %s", configs.Data.MS["cna"].Title, a.U.String())
	}
	metas := exhtml.MetasByItemprop(a.doc, "dateModified")
	cs := []string{}
	for _, meta := range metas {
		for _, a := range meta.Attr {
			if a.Key == "content" {
				cs = append(cs, a.Val)
			}
		}
	}
	if len(cs) <= 0 {
		return nil, fmt.Errorf("[%s] no date extracted: %s",
			configs.Data.MS["cna"].Title, a.U.String())
	}
	// China doesn't have daylight saving. It uses a fixed 8 hour offset from UTC.
	t, err := time.Parse("2006/01/02 15:04", cs[0])
	if err != nil {
		return nil, err
	}
	return timestamppb.New(t), nil
}

func shanghai(t time.Time) time.Time {
	loc := time.FixedZone("UTC", 8*60*60)
	return t.In(loc)
}

func (a *Article) fetchContent() (string, error) {
	if a.doc == nil {
		return "", errors.Errorf("[%s] fetchContent: doc is nil: %s", configs.Data.MS["cna"].Title, a.U.String())
	}
	doc := a.doc
	body := ""
	// Fetch content nodes
	nodes := exhtml.ElementsByTagAndClass(doc, "div", "paragraph")
	if len(nodes) == 0 {
		return "", fmt.Errorf("[%s] There is no element class is paragraph` from: %s",
			configs.Data.MS["cna"].Title, a.U.String())
	}
	n := nodes[0]
	plist := exhtml.ElementsByTag(n, "h2", "p")
	for _, v := range plist {
		if v.FirstChild != nil {
			body += v.FirstChild.Data + "  \n"
		}
	}
	replace := func(src, x, y string) string {
		re := regexp.MustCompile(x)
		return re.ReplaceAllString(src, y)
	}

	body = replace(body, "???", "???")
	body = replace(body, "???", "???")
	body = replace(body, "</a>", "")
	body = replace(body, `<a.*?>`, "")
	body = replace(body, `<script.*?</script>`, "")
	body = replace(body, `<iframe.*?</iframe>`, "")

	return body, nil
}

func (a *Article) fmtContent(body string) (string, error) {
	var err error
	title := "# " + a.Title + "\n\n"
	lastupdate := a.UpdateTime.AsTime().Format(time.RFC3339)
	webTitle := fmt.Sprintf(" @ [%s](/list/?v=%[1]s): [%[2]s](http://%[2]s)", a.WebsiteTitle, a.WebsiteDomain)
	u, err := url.QueryUnescape(a.U.String())
	if err != nil {
		u = a.U.String() + "\n\nunescape url error:\n" + err.Error()
	}

	body = title +
		"LastUpdate: " + lastupdate +
		webTitle + "\n\n" +
		"---\n" +
		body + "\n\n" +
		"????????????" + fmt.Sprintf("[%s](%[1]s)", u)
	return body, nil
}
