package model

import (
	"errors"
	"net/url"
)

type URLShortenPostRequest struct {
	URL string `json:"url"`
}

var ErrEmptyOrInvalidURL = errors.New("invalid or missing original URL")

func (r *URLShortenPostRequest) Validate() error {
	if r.URL == "" {
		return ErrEmptyOrInvalidURL
	}
	_, err := url.ParseRequestURI(r.URL)
	return err
}