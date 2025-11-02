package untappd

import "fmt"

const VenueUntappdAtHome = "Untappd at Home"

type UntappdResponse struct {
	Response Response `json:"response"`
}

type Response struct {
	Checkins   Checkins   `json:"checkins"`
	Pagination Pagination `json:"pagination"`
}

type Checkins struct {
	Items []Checkin `json:"items"`
}

type Pagination struct {
	SinceURL string `json:"since_url"`
}

type Checkin struct {
	CheckinID      int     `json:"checkin_id"`
	CheckinComment string  `json:"checkin_comment"`
	RatingScore    float64 `json:"rating_score"`
	CreatedAt      string  `json:"created_at"`
	Media          Media   `json:"media"`
	Beer           Beer    `json:"beer"`
	Brewery        Brewery `json:"brewery"`
	Venue          *Venue  `json:"venue"`
}

type Media struct {
	Items []MediaItem `json:"items"`
}

type MediaItem struct {
	Photo Photo `json:"photo"`
}

type Photo struct {
	PhotoImgOg string `json:"photo_img_og"`
}

type Beer struct {
	BeerName  string  `json:"beer_name"`
	BeerStyle string  `json:"beer_style"`
	BeerABV   float64 `json:"beer_abv"`
}

type Brewery struct {
	BreweryName    string `json:"brewery_name"`
	BreweryCountry string `json:"country_name"`
}

type Venue struct {
	VenueName string   `json:"venue_name"`
	Location  Location `json:"location"`
}

type Location struct {
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
	City    string  `json:"venue_city"`
	State   string  `json:"venue_state"`
	Country string  `json:"venue_country"`
}

func (v *Venue) Name() string {
	if v == nil {
		return ""
	}
	return v.VenueName
}

func (v *Venue) City() string {
	if v == nil || v.VenueName == VenueUntappdAtHome {
		return ""
	}
	return v.Location.City
}

func (v *Venue) State() string {
	if v == nil || v.VenueName == VenueUntappdAtHome {
		return ""
	}
	return v.Location.State
}

func (v *Venue) Country() string {
	if v == nil || v.VenueName == VenueUntappdAtHome {
		return ""
	}
	return v.Location.Country
}

func (v *Venue) LatLng() string {
	if v == nil || v.VenueName == VenueUntappdAtHome {
		return ""
	}
	return fmt.Sprintf("%f,%f", v.Location.Lat, v.Location.Lng)
}
