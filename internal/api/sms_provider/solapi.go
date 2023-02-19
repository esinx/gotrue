package sms_provider

import (
	"fmt"
	"net/url"

	"github.com/solapi/solapi-go"
	"github.com/supabase/gotrue/internal/conf"
)

const (
	defaultSolapiApiBase = "https://api.solapi.com"
)

type SolapiProvider struct {
	Config  *conf.SolapiProviderConfiguration
	APIPath string
}

func NewSolapiProvider(config conf.SolapiProviderConfiguration) (SmsProvider, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	apiPath := defaultSolapiApiBase
	return &SolapiProvider{
		Config:  &config,
		APIPath: apiPath,
	}, nil
}

func (t *SolapiProvider) SendSms(phone string, message string) error {
	client := solapi.NewClient()

	serverURL, err := url.Parse(t.APIPath)

	fmt.Printf("%s", client.Messages.Config)

	// client.Messages.Config["Protocol"] = serverURL.Scheme
	// client.Messages.Config["Domain"] = serverURL.Host
	// client.Messages.Config["APIKey"] = t.Config.ApiKey
	// client.Messages.Config["APISecret"] = t.Config.ApiSecret

	body := make(map[string]interface{})
	body["to"] = phone
	body["from"] = t.Config.From
	body["text"] = message
	body["type"] = "SMS"
	body["Protocol"] = serverURL.Scheme
	body["Domain"] = serverURL.Host
	body["APIKey"] = t.Config.ApiKey
	body["APISecret"] = t.Config.ApiSecret

	params := make(map[string]interface{})
	params["message"] = body

	result, err := client.Messages.SendSimpleMessage(params)

	if err != nil {
		return err
	}

	fmt.Printf("%s", result.StatusCode)

	return nil
}
