package services

import (
	"encoding/json"
	"fmt"
	"github.com/openneo/neopia/models"
	"net/http"
	"net/url"
)

type ImpressClient struct {
	submitURL string
}

func NewImpressClient(host string) ImpressClient {
	return ImpressClient{submitURL: fmt.Sprintf("%s/pets/submit.json", host)}
}

func (impress ImpressClient) Submit(c models.Customization, userId int) (*http.Response, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	v := url.Values{}
	v.Set("viewer_data", string(b))
	if userId > 0 {
		v.Set("user_id", fmt.Sprintf("%d", userId))
	}
	return http.PostForm(impress.submitURL, v)
}
