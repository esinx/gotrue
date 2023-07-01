package sms_provider

import (
	"bytes"
	"crypto/hmac"
	cr "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/supabase/gotrue/internal/conf"
	"github.com/supabase/gotrue/internal/utilities"
)

const (
	defaultSolapiApiBase = "https://api.solapi.com"
)

type SolapiProvider struct {
	Config  *conf.SolapiProviderConfiguration
	APIPath string
}

type SolapiError struct {
	ErrorCode    string
	ErrorMessage string
}

type SolapiMessage struct {
	GroupId       string
	MessageId     string
	AccountId     string
	StatusMessage string
	StatusCode    string
	To            string
	From          string
	Type          string
	Country       string
}

func NewSolapiProvider(config conf.SolapiProviderConfiguration) (SmsProvider, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	apiPath := defaultSolapiApiBase + "/messages/v4/send"
	return &SolapiProvider{
		Config:  &config,
		APIPath: apiPath,
	}, nil
}

const countrycode = `^(?P<code>670|239|993|262|370|855|242|41|251|297|268|54|591|237|226|973|966|596|691|81|238|386|502|387|965|962|1767|231|960|1876|968|255|84|225|299|241|508|377|681|64|967|376|355|685|853|971|1671|383|91|994|266|1340|254|992|90|93|880|677|1649|1758|378|689|33|1441|421|252|51|856|674|248|47|265|682|229|382|1869|228|86|374|1809|380|233|676|1345|218|358|236|230|423|61|223|7|359|1|40|244|235|27|357|599|46|974|60|43|258|256|36|227|211|55|298|224|507|506|352|1684|1242|1784|350|353|92|680|234|593|1670|673|375|98|213|503|420|692|56|1787|32|66|509|501|852|232|995|220|63|373|212|385|976|245|686|264|1473|964|351|372|598|52|961|998|216|253|250|1268|34|57|257|886|679|1246|261|39|599|975|249|977|356|243|31|597|1264|58|972|62|354|260|221|675|1868|263|49|678|45|7|48|222|996|970|269|1664|687|389|94|371|1721|592|963|590|504|95|240|20|505|65|381|267|44|1284|30|595|594|269|82)(?P<num>\d+)$`

func ParsePhoneNumber(phone string) (string, string) {
	exp := regexp.MustCompile(countrycode)
	match := exp.FindStringSubmatch(phone)
	return match[1], match[2]
}

func (t *SolapiProvider) SendMessage(phone string, message string, channel string) (string, error) {
	switch channel {
	case SMSProvider:
		return t.SendSms(phone, message)
	default:
		return "", fmt.Errorf("channel type %q is not supported for Solapi", channel)
	}
}

func (t *SolapiProvider) SendSms(phone string, message string) (string, error) {
	country, phone := ParsePhoneNumber(phone)
	client := &http.Client{Timeout: defaultTimeout}
	body := map[string]map[string]interface{}{
		"message": {
			"to":      phone,
			"from":    t.Config.From,
			"text":    message,
			"type":    "SMS",
			"country": country,
		},
	}
	jsonString, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	request, err := http.NewRequest("POST", t.APIPath, bytes.NewBuffer(jsonString))
	if err != nil {
		return "", err
	}
	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("Authorization", t.CreateAuthHeader())
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}

	if response.StatusCode != 200 {
		errorStruct := SolapiError{}
		err = json.NewDecoder(response.Body).Decode(&errorStruct)
		if err != nil {
			return "", err
		}
		errString := fmt.Sprintf("%s[%d]:%s", errorStruct.ErrorCode, response.StatusCode, errorStruct.ErrorMessage)
		return "", errors.New(errString)
	}

	customStruct := SolapiMessage{}
	err = json.NewDecoder(response.Body).Decode(&customStruct)
	if err != nil {
		return "", err
	}

	defer utilities.SafeClose(response.Body)

	return customStruct.MessageId, nil
}

// RandomString returns a random string
func RandomString(n int) string {
	b := make([]byte, n)
	_, _ = cr.Read(b)
	return hex.EncodeToString(b)
}

func (t *SolapiProvider) CreateAuthHeader() string {
	salt := RandomString(20)
	date := time.Now().Format(time.RFC3339)
	h := hmac.New(sha256.New, []byte(t.Config.ApiSecret))
	h.Write([]byte(date + salt))
	signature := hex.EncodeToString(h.Sum(nil))
	authorization := fmt.Sprintf("HMAC-SHA256 apiKey=%s, date=%s, salt=%s, signature=%s", t.Config.ApiKey, date, salt, signature)
	return authorization
}
