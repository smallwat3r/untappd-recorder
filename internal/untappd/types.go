package untappd

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
	ServingStyle   string  `json:"serving_style"`
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
	BreweryName string `json:"brewery_name"`
}

type Venue struct {
	VenueName string   `json:"venue_name"`
	Location  Location `json:"location"`
}

type Location struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}
