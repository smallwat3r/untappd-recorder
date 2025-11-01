package untappd

type UntappdResponse struct {
	Response struct {
		Checkins struct {
			Items []Checkin `json:"items"`
		} `json:"checkins"`
	} `json:"response"`
}

type Checkin struct {
	CheckinComment string `json:"checkin_comment"`
	RatingScore    float64 `json:"rating_score"`
	Media          struct {
		Items []struct {
			Photo struct {
				PhotoImgOg string `json:"photo_img_og"`
			} `json:"photo"`
		} `json:"items"`
	} `json:"media"`
	Beer struct {
		BeerName string `json:"beer_name"`
	} `json:"beer"`
	Brewery struct {
		BreweryName string `json:"brewery_name"`
	} `json:"brewery"`
	Venue struct {
		VenueName string `json:"venue_name"`
		Location  struct {
			Lat float64 `json:"lat"`
			Lng float64 `json:"lng"`
		} `json:"location"`
	} `json:"venue"`
}
