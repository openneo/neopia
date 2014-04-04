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

func servePublicJSONBytes(w http.ResponseWriter, r *http.Request, b []byte, status int) {
	// Allow any website to send requests for this resource - it *is* public
	// JSON, after all.
	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.WriteHeader(status)

	callback := r.FormValue("callback")
	if callback == "" {
		fmt.Fprintf(w, "%s", b)
	} else {
		fmt.Fprintf(w, "%s(%s);", callback, b)
	}
}

func servePublicJSON(w http.ResponseWriter, r *http.Request, v interface{}) {
	b, err := json.Marshal(v)
	if err != nil {
		servePublicJSONError(w, r, err, http.StatusInternalServerError)
		return
	}
	servePublicJSONBytes(w, r, b, http.StatusOK)
}

func servePublicJSONErrorMessage(w http.ResponseWriter, r *http.Request, msg string, status int) {
	b := []byte(fmt.Sprintf("{\"error\": %s}", strconv.Quote(msg)))
	servePublicJSONBytes(w, r, b, status)
}

func servePublicJSONError(w http.ResponseWriter, r *http.Request, err error, status int) {
	servePublicJSONErrorMessage(w, r, err.Error(), status)
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

func redirectToImpress(w http.ResponseWriter, r *http.Request, petName string, impressHost string, destination string) {
	redirectUrl := "http://" + impressHost + "/pets/load?name=" +
		url.QueryEscape(petName) + "&destination=" + destination
	http.Redirect(w, r, redirectUrl, http.StatusTemporaryRedirect)
}

func serveCustomization(w http.ResponseWriter, r *http.Request, cc chan customizationSubmission, petName string, impressHost string) {
	if petName[0] >= '0' && petName[0] <= '9' {
		// The JSON endpoint thinks that names that start with digits are
		// integers and times out on them. They get special treatment.
		// If it's a request to DTI, pass them off to the old embedded AMF pet
		// loader. Otherwise, yield a JSON error.
		redirectFormat := r.FormValue("redirect")
		if redirectFormat == "http://"+impressHost+"/wardrobe#{q}" {
			redirectToImpress(w, r, petName, impressHost, "wardrobe")
		} else if redirectFormat == "http://"+impressHost+"/#{q}" {
			redirectToImpress(w, r, petName, impressHost, "")
		} else {
			servePublicJSONErrorMessage(w, r,
				"pet names with leading digits are unsupported",
				http.StatusBadRequest)
		}
		return
	}

	// Get customization
	c, err := models.GetCustomization(petName)
	if err != nil {
		servePublicJSONError(w, r, err, http.StatusInternalServerError)
		return
	}

	// Serve cache headers
	writeExpiresIn(w, time.Duration(5)*time.Minute, time.Now())

	// Serve customization
	redirectFormat := r.FormValue("redirect")
	if redirectFormat == "" { // serve as JSON
		servePublicJSON(w, r, c)
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
		_, isNotFound := err.(models.UserNotFoundError)
		if isNotFound {
			servePublicJSONError(w, r, err, http.StatusNotFound)
		} else {
			servePublicJSONError(w, r, err, http.StatusInternalServerError)
		}
		return
	}
	// Fun fact, since I was worried about this: if you send cache headers
	// alongside a POST request, future POST requests still will not be served
	// from the cache. Instead, the new response will be cached and served in
	// response to future GET requests, which is exactly what we want here: the
	// ability to semantically assert that the pet has changed and needs to be
	// reexamined via POST, while still serving cached results from GET in less
	// urgent scenarios.
	// http://lists.w3.org/Archives/Public/ietf-http-wg/2008OctDec/0200.html
	writeExpiresIn(w, time.Duration(1)*time.Hour, time.Now())
	servePublicJSON(w, r, usersResponse{
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
		serveCustomization(w, r, csc, r.FormValue("name"), *impressHost)
	})
	http.HandleFunc("/api/1/pets/", func(w http.ResponseWriter, r *http.Request) {
		// 0:/1:api/2:1/3:pets/4:thyassa/5:customization
		components := strings.Split(r.URL.Path, "/")
		if len(components) != 6 || components[5] != "customization" {
			http.NotFound(w, r)
			return
		}
		serveCustomization(w, r, csc, components[4], *impressHost)
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
