package model

type ErrorResponse struct {
	Error ErrorResponseError `json:"error"`
}

type ErrorResponseError struct {
	Code string `json:"code"`
	Message string `json:"message"`
}