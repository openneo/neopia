package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/openneo/neopia/models"
	"github.com/openneo/neopia/services"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type customizationSubmission struct {
	Customization models.Customization
	ImpressUserId int
}

type usersResponse struct {
	Users []userResponse `json:"users"`
}

type userResponse struct {
	Id    string           `json:"id"`
	Links petLinksResponse `json:"links"`
}

type petLinksResponse struct {
	Pets []string `json:"pets"`
}

func serveJSONBytes(w http.ResponseWriter, r *http.Request, b []byte) {
	callback := r.FormValue("callback")
	if callback == "" {
		fmt.Fprintf(w, "%s", b)
	} else {
		fmt.Fprintf(w, "%s(%s);", callback, b)
	}
}

func serveJSON(w http.ResponseWriter, r *http.Request, v interface{}) {
	b, err := json.Marshal(v)
	if err != nil {
		serveJSONError(w, r, err)
		return
	}
	serveJSONBytes(w, r, b)
}

func serveJSONError(w http.ResponseWriter, r *http.Request, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	b := []byte(fmt.Sprintf("{error: %s}", strconv.Quote(err.Error())))
	serveJSONBytes(w, r, b)
}

func writeExpiresIn(w http.ResponseWriter, timeUntilExpiry time.Duration,
	now time.Time) {
	writeExpiryHeaders(w, now.Add(timeUntilExpiry), timeUntilExpiry)
}

func writeExpiryHeaders(w http.ResponseWriter, expiry time.Time,
	timeUntilExpiry time.Duration) {
	secondsUntilExpiry := int(timeUntilExpiry.Seconds())
	if secondsUntilExpiry < 0 {
		secondsUntilExpiry = 0
	}

	w.Header().Add("cache-control",
		fmt.Sprintf("public, max-age=%d", secondsUntilExpiry))
	w.Header().Add("expires", expiry.Format(time.RFC1123))
}

func serveCustomization(w http.ResponseWriter, r *http.Request, cc chan customizationSubmission, petName string) {
	// Get customization
	c, err := models.GetCustomization(petName)
	if err != nil {
		serveJSONError(w, r, err)
		return
	}

	// Serve cache headers
	writeExpiresIn(w, time.Duration(5)*time.Minute, time.Now())

	// Serve customization
	redirectFormat := r.FormValue("redirect")
	if redirectFormat == "" { // serve as JSON
		serveJSON(w, r, c)
	} else { // serve as redirect with query string
		v := url.Values{}
		v.Set("name", c.CustomPet.Name)
		v.Set("color", fmt.Sprintf("%d", c.CustomPet.ColorId))
		v.Set("species", fmt.Sprintf("%d", c.CustomPet.SpeciesId))
		for _, b := range c.CustomPet.BiologyByZone {
			v.Add("biology[]", fmt.Sprintf("%d", b.PartId))
		}
		for _, oi := range c.ObjectInfoRegistry {
			v.Add("objects[]", fmt.Sprintf("%d", oi.Id))
		}
		redirectUrl := strings.Replace(redirectFormat, "{q}", v.Encode(), -1)
		http.Redirect(w, r, redirectUrl, http.StatusTemporaryRedirect)
	}

	// Submit customization
	impressUserId, err := strconv.ParseInt(r.FormValue("impress_user"), 10, 0)
	if err != nil {
		impressUserId = -1
	}
	cc <- customizationSubmission{c, int(impressUserId)}
}

func serveUser(w http.ResponseWriter, r *http.Request, name string) {
	u, err := models.GetUser(name)
	if err != nil {
		serveJSONError(w, r, err)
		return
	}
	writeExpiresIn(w, time.Duration(1)*time.Hour, time.Now())
	serveJSON(w, r, usersResponse{
		[]userResponse{userResponse{u.Name, petLinksResponse{u.PetNames}}},
	})
}

func submit(impress services.ImpressClient, csc chan customizationSubmission) {
	for {
		cs := <-csc
		resp, err := impress.Submit(cs.Customization, cs.ImpressUserId)
		if err != nil {
			log.Printf("impress failed: %s", err)
			return
		}
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Printf("can't read impress: %s", err)
		}
		resp.Body.Close()
		log.Printf("impress responded: %s", body)
	}
}

func main() {
	port := flag.Int("port", 8888, "port on which to run web server")
	impressHost := flag.String("impress", "impress.openneo.net", "Dress to Impress host")
	flag.Parse()

	impress := services.NewImpressClient(*impressHost)
	csc := make(chan customizationSubmission, 32)
	go submit(impress, csc)

	http.HandleFunc("/api/1/pet/customization", func(w http.ResponseWriter, r *http.Request) {
		serveCustomization(w, r, csc, r.FormValue("name"))
	})
	http.HandleFunc("/api/1/pets/", func(w http.ResponseWriter, r *http.Request) {
		// 0:/1:api/2:1/3:pets/4:thyassa/5:customization
		components := strings.Split(r.URL.Path, "/")
		if len(components) != 6 || components[5] != "customization" {
			http.NotFound(w, r)
			return
		}
		serveCustomization(w, r, csc, components[4])
	})
	http.HandleFunc("/api/1/users/", func(w http.ResponseWriter, r *http.Request) {
		// 0:/1:api/2:1/3:users/4:borovan
		components := strings.Split(r.URL.Path, "/")
		if len(components) != 5 {
			http.NotFound(w, r)
			return
		}
		serveUser(w, r, components[4])
	})
	http.ListenAndServe(fmt.Sprintf(":%d", *port), nil)
}
