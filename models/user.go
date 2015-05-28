package models

import (
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"net/url"
	"strings"
)

type User struct {
	Name     string
	PetNames []string
}

type UserNotFoundError struct {
	Name string
}

func (e UserNotFoundError) Error() string {
	return fmt.Sprintf("user %s not found", e.Name)
}

func GetUser(name string) (User, error) {
	user := User{}

	lookupURL := fmt.Sprintf("http://www.neopets.com/userlookup.phtml?user=%s",
		url.QueryEscape(name))
	doc, err := goquery.NewDocument(lookupURL)
	if err != nil {
		return user, err
	}

	petsWrappers := doc.Find("#userneopets")
	if petsWrappers.Length() == 0 {
		return user, UserNotFoundError{name}
	}
	petsWrapper := petsWrappers.Last()
	petNodes := petsWrapper.Find("a[href^='/petlookup.phtml?pet=']")
	petNodes.Each(func(_ int, s *goquery.Selection) {
		href, ok := s.Attr("href")
		if ok {
			url, err := url.Parse(href)
			if err == nil {
				petName := url.Query().Get("pet")
				if petName != "" {
					user.PetNames = append(user.PetNames, petName)
				}
			}
		}
	})

	user.Name = strings.ToLower(strings.TrimSpace(name))
	return user, nil
}
