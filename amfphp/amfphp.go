package amfphp

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
)

type RemoteGateway struct {
	url string
}

type RemoteService struct {
	gateway RemoteGateway
	name    string
}

type RemoteMethod struct {
	service RemoteService
	name    string
	responseIsPresent func([]byte) bool
}

func NewRemoteGateway(url string) RemoteGateway {
	return RemoteGateway{url: url}
}

func (g RemoteGateway) Service(name string) RemoteService {
	return RemoteService{gateway: g, name: name}
}

func (s RemoteService) Method(name string, responseIsPresent func([]byte) bool) RemoteMethod {
	return RemoteMethod{service: s, name: name, responseIsPresent: responseIsPresent}
}

func (m RemoteMethod) Call(dest interface{}, args ...string) (present bool, err error) {
	var urlBuffer bytes.Buffer
	urlBuffer.WriteString(m.service.gateway.url)
	urlBuffer.WriteString("/")
	urlBuffer.WriteString(m.service.name)
	urlBuffer.WriteString(".")
	urlBuffer.WriteString(m.name)
	for _, arg := range args {
		urlBuffer.WriteString("/")
		urlBuffer.WriteString(url.QueryEscape(arg))
	}
	url := urlBuffer.String()

	resp, err := http.Get(url)
	if err != nil {
		return false, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}
	resp.Body.Close()

	if !m.responseIsPresent(body) {
		return false, nil
	}

	err = json.Unmarshal(body, dest)
	return true, err
}
