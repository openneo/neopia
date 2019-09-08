package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/openneo/neopia/amfphp"
	"github.com/openneo/neopia/models"
	"github.com/openneo/neopia/services"
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

type statusResponse struct {
	Status bool `json:"status"`
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
	redirectUrl := impressHost + "/pets/load?name=" +
		url.QueryEscape(petName) + "&destination=" + destination
	http.Redirect(w, r, redirectUrl, http.StatusTemporaryRedirect)
}

func serveCustomizationErrorMessage(w http.ResponseWriter, r *http.Request, msg string, status int, petName string) {
	if r.FormValue("redirect") == "" {
		servePublicJSONErrorMessage(w, r, msg, status)
	} else {
		referer, err := url.ParseRequestURI(r.Referer())
		if err != nil {
			servePublicJSONErrorMessage(w, r, msg, status)
			return
		}
		q := referer.Query()
		q.Set("name", petName)
		q.Set("neopia[error]", msg)
		referer.RawQuery = q.Encode()
		http.Redirect(w, r, referer.String(), http.StatusTemporaryRedirect)
	}
}

func serveCustomization(w http.ResponseWriter, r *http.Request, cc chan customizationSubmission, petName string, customizationService models.CustomizationService, impressHost string) {
	if len(petName) == 0 {
		serveCustomizationErrorMessage(w, r, "name blank",
			http.StatusBadRequest, petName)
		return
	}

	if petName[0] >= '0' && petName[0] <= '9' {
		// The JSON endpoint thinks that names that start with digits are
		// integers and times out on them. They get special treatment.
		// If it's a request to DTI, pass them off to the old embedded AMF pet
		// loader. Otherwise, yield a JSON error.
		redirectFormat := r.FormValue("redirect")
		if redirectFormat == impressHost+"/wardrobe#{q}" {
			redirectToImpress(w, r, petName, impressHost, "wardrobe")
		} else if redirectFormat == impressHost+"/#{q}" {
			redirectToImpress(w, r, petName, impressHost, "")
		} else {
			serveCustomizationErrorMessage(w, r,
				"pet names with leading digits are unsupported",
				http.StatusBadRequest, petName)
		}
		return
	}

	// Get customization
	c, found, err := customizationService.GetCustomization(petName)
	if err != nil {
		serveCustomizationErrorMessage(w, r, err.Error(),
			http.StatusInternalServerError, petName)
		return
	}

	// Serve cache headers
	// Fun fact, since I was worried about this: if you send cache headers
	// alongside a POST request, future POST requests still will not be served
	// from the cache. Instead, the new response will be cached and served in
	// response to future GET requests, which is exactly what we want here: the
	// ability to semantically assert that the pet has changed and needs to be
	// reexamined via POST, while still serving cached results from GET in less
	// urgent scenarios.
	// http://lists.w3.org/Archives/Public/ietf-http-wg/2008OctDec/0200.html
	writeExpiresIn(w, time.Duration(5)*time.Minute, time.Now())

	if !found {
		serveCustomizationErrorMessage(w, r, "pet not found",
			http.StatusNotFound, petName)
		return
	}

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

func serveUser(w http.ResponseWriter, r *http.Request, userService models.UserService, name string) {
	u, found, err := userService.GetUser(name)
	if !found {
		servePublicJSONErrorMessage(w, r, fmt.Sprintf("user \"%s\" not found", name), http.StatusNotFound)
	} else if err != nil {
		servePublicJSONError(w, r, err, http.StatusInternalServerError)
	} else {
		writeExpiresIn(w, time.Duration(1)*time.Hour, time.Now())
		servePublicJSON(w, r, usersResponse{
			[]userResponse{userResponse{u.Name, petLinksResponse{u.PetNames}}},
		})
	}
}

func serveStatus(w http.ResponseWriter, r *http.Request, status bool) {
	servePublicJSON(w, r, statusResponse{status})
}

func ping(client http.Client, url string) bool {
	resp, err := client.Get(url)

	status := (err == nil)
	if err != nil {
		log.Print("Neopets.com status: false. Network request failed.")
		return false
	}

	defer resp.Body.Close()

	// NOTE(matchu): It was hard to pick a pattern that felt like a reliable
	//     indicator that it's the real homepage, while still being relatively
	//     resilient to refactors. But I figure that some concept of "login" or
	//     "log in" will be pretty stable? (Though if they change their
	//     language to "sign in", then, well, heck :p)
	containsLogin, err := regexp.MatchReader("(?i)log ?in", bufio.NewReader(resp.Body))
	if err != nil {
		log.Print("Neopets.com status: false. Reading body failed.")
		return false
	}
	if !containsLogin {
		log.Print("Neopets.com status: false. Body exists, but didn't contain \"login\".")
		return false
	}

	log.Print("Neopets.com status: true. All OK!")
	return status
}

func submit(impress services.ImpressClient, csc chan customizationSubmission) {
	for {
		cs := <-csc
		resp, err := impress.Submit(cs.Customization, cs.ImpressUserId)
		if err != nil {
			log.Printf("impress failed: %s", err)
			return
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Printf("can't read impress: %s", err)
		}
		log.Printf("impress responded: %s", body)
	}
}

func main() {
	port := flag.Int("port", 8888, "port on which to run web server")
	neopetsHost := flag.String("neopetsGateway", "http://www.neopets.com/amfphp/json.php", "Neopets JSON gateway URL")
	impressHost := flag.String("impress", "https://impress.openneo.net", "Dress to Impress host")
	pingIntervalInSeconds := flag.Int("pingInterval", 60, "Ping Neopets for status every X seconds")
	pingTimeoutInSeconds := flag.Int("pingTimeout", 10, "Give up on the Neopets ping after X seconds")
	pingUrl := flag.String("pingUrl", "http://www.neopets.com/", "Neopets URL to ping")
	flag.Parse()

	neopetsGateway := amfphp.NewRemoteGateway(*neopetsHost)
	customizationService := models.NewCustomizationService(neopetsGateway)
	userService := models.NewUserService(neopetsGateway)

	impress := services.NewImpressClient(*impressHost)
	csc := make(chan customizationSubmission, 32)
	go submit(impress, csc)

	ticker := time.NewTicker(time.Duration(*pingIntervalInSeconds) * time.Second)

	pingTimeout := time.Duration(time.Duration(*pingTimeoutInSeconds) * time.Second)
	pingClient := http.Client{Timeout: pingTimeout}
	status := true
	go func() {
		for {
			_ = <-ticker.C
			status = ping(pingClient, *pingUrl)
		}
	}()

	http.HandleFunc("/api/1/status", func(w http.ResponseWriter, r *http.Request) {
		serveStatus(w, r, status)
	})
	http.HandleFunc("/api/1/pet/customization", func(w http.ResponseWriter, r *http.Request) {
		serveCustomization(w, r, csc, r.FormValue("name"), customizationService, *impressHost)
	})
	http.HandleFunc("/api/1/pets/", func(w http.ResponseWriter, r *http.Request) {
		// 0:/1:api/2:1/3:pets/4:thyassa/5:customization
		components := strings.Split(r.URL.Path, "/")
		if len(components) != 6 || components[5] != "customization" {
			http.NotFound(w, r)
			return
		}
		serveCustomization(w, r, csc, components[4], customizationService, *impressHost)
	})
	http.HandleFunc("/api/1/users/", func(w http.ResponseWriter, r *http.Request) {
		// 0:/1:api/2:1/3:users/4:borovan
		components := strings.Split(r.URL.Path, "/")
		if len(components) != 5 {
			http.NotFound(w, r)
			return
		}
		serveUser(w, r, userService, components[4])
	})
	http.ListenAndServe(fmt.Sprintf(":%d", *port), nil)
}
