package conflate

import (
	"io/ioutil"
	"net"
	"net/http"
	pkgurl "net/url"
	"os"
	"time"
)

var emptyURL = pkgurl.URL{}
var getwd = os.Getwd

func loadURLs(urls ...pkgurl.URL) ([][]byte, error) {
	var allData [][]byte
	for _, url := range urls {
		data, err := loadURL(url)
		if err != nil {
			return nil, err
		}
		allData = append(allData, data)
	}
	return allData, nil
}

func loadURL(url pkgurl.URL) ([]byte, error) {
	client := http.Client{Transport: newTransport()}
	resp, err := client.Get(url.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, makeError("Failed to load url : %v : %v", resp.StatusCode, url.String())
	}
	return data, err
}

func loadURLsRecursive(parentUrls []pkgurl.URL, urls ...pkgurl.URL) ([][]byte, error) {
	var allData [][]byte
	for _, url := range urls {
		data, err := loadURLRecursive(parentUrls, url)
		if err != nil {
			return nil, err
		}
		allData = append(allData, data...)
	}
	return allData, nil
}

func loadURLRecursive(parentUrls []pkgurl.URL, url pkgurl.URL) ([][]byte, error) {
	data, err := loadURL(url)
	if err != nil {
		return nil, err
	}
	return loadDatumRecursive(parentUrls, &url, data)
}

func loadDataRecursive(parentUrls []pkgurl.URL, data ...[]byte) ([][]byte, error) {
	var allData [][]byte
	for _, datum := range data {
		childData, err := loadDatumRecursive(parentUrls, nil, datum)
		if err != nil {
			return nil, err
		}
		allData = append(allData, childData...)
	}
	return allData, nil
}

func loadDatumRecursive(parentUrls []pkgurl.URL, url *pkgurl.URL, data []byte) ([][]byte, error) {
	if containsURL(url, parentUrls) {
		return nil, makeError("The url recursively includes itself (%v)", url)
	}
	childPaths, err := extractIncludes(data)
	if err != nil {
		return nil, err
	}
	childUrls, err := toURLs(url, childPaths...)
	if err != nil {
		return nil, err
	}
	var newParentUrls []pkgurl.URL
	newParentUrls = append(newParentUrls, parentUrls...)
	if url != nil {
		newParentUrls = append(newParentUrls, *url)
	}
	childData, err := loadURLsRecursive(newParentUrls, childUrls...)
	if err != nil {
		return nil, err
	}
	var allData [][]byte
	allData = append(allData, childData...)
	allData = append(allData, data)
	return allData, nil
}

func containsURL(searchURL *pkgurl.URL, urls []pkgurl.URL) bool {
	if searchURL == nil {
		return false
	}
	for _, url := range urls {
		if url == *searchURL {
			return true
		}
	}
	return false
}

func newClient() http.Client {
	return http.Client{Transport: newTransport()}
}

func newTransport() *http.Transport {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	transport.RegisterProtocol("file", http.NewFileTransport(http.Dir("/")))
	return transport
}

func toURLs(rootURL *pkgurl.URL, paths ...string) ([]pkgurl.URL, error) {
	var urls []pkgurl.URL
	for _, path := range paths {
		url, err := toURL(rootURL, path)
		if err != nil {
			return nil, err
		}
		urls = append(urls, url)
	}
	return urls, nil
}

func toURL(rootURL *pkgurl.URL, path string) (pkgurl.URL, error) {
	var err error
	if rootURL == nil {
		rootURL, err = workingDir()
		if err != nil {
			return emptyURL, err
		}
	}
	url, err := pkgurl.Parse(path)
	if err != nil {
		return emptyURL, wrapError(err, "Could not parse path")
	}
	if !url.IsAbs() {
		url = rootURL.ResolveReference(url)
		url.RawQuery = rootURL.RawQuery
	}
	return *url, nil
}

func extractIncludes(data []byte) ([]string, error) {
	out := struct {
		Includes []string
	}{}
	err := unmarshalAny(data, &out)
	if err != nil {
		return nil, wrapError(err, "Could not extract includes")
	}
	return out.Includes, nil
}

func workingDir() (*pkgurl.URL, error) {
	rootPath, err := getwd()
	if err != nil {
		return nil, err
	}
	rootURL, err := pkgurl.Parse("file://" + rootPath + "/")
	if err != nil {
		return nil, err
	}
	return rootURL, nil
}
