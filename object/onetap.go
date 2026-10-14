// Copyright 2026 The Casdoor Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package object

import (
	"errors"
	"fmt"
	"strings"

	"github.com/casdoor/casdoor/i18n"
	"github.com/casdoor/casdoor/util"
)

// OneTapProviderType is the provider Type that carries the Aliyun number-
// authentication (号码认证服务 / Dypns) credentials. One-tap login reuses this
// provider's AccessKey pair and region; the same provider type already backs
// PNVS SMS verification codes.
const OneTapProviderType = "Alibaba Cloud PNVS SMS"

// GetOneTapProvider returns the application's provider that holds the number-
// authentication credentials. It is resolved from the application (never named
// by the caller) so one app can never reach another organization's credentials.
func GetOneTapProvider(application *Application, lang string) (*Provider, error) {
	if application == nil {
		return nil, errors.New(i18n.Translate(lang, "auth:The application does not exist"))
	}

	providers, err := GetProviders(application.Organization)
	if err != nil {
		return nil, err
	}

	enabled := map[string]bool{}
	for _, providerItem := range application.Providers {
		enabled[providerItem.Name] = true
	}

	for _, provider := range providers {
		if provider.Type == OneTapProviderType && enabled[provider.Name] {
			return provider, nil
		}
	}

	return nil, errors.New(i18n.Translate(lang, "provider:The one-tap (number authentication) provider is not configured for this application"))
}

// GetPhoneByOneTapToken exchanges the carrier access token for the verified
// phone number by calling Aliyun GetMobile with the provider's AccessKey pair.
// This is the one step that cannot move to the client.
func GetPhoneByOneTapToken(provider *Provider, accessToken string, lang string) (string, error) {
	if provider == nil || provider.Type != OneTapProviderType {
		return "", errors.New(i18n.Translate(lang, "provider:The one-tap (number authentication) provider is not configured for this application"))
	}
	if accessToken == "" {
		return "", errors.New(i18n.Translate(lang, "general:Missing parameter") + ": accessToken")
	}

	client, err := newPnvsSmsClient(provider.ClientId, provider.ClientSecret, provider.SignName, provider.TemplateCode, provider.RegionId)
	if err != nil {
		return "", err
	}

	mobile, err := client.GetMobile(accessToken)
	if err != nil {
		return "", err
	}

	// Aliyun returns a bare national number; keep only the national part so it
	// matches how phone numbers are stored (national number + country code).
	mobile = strings.TrimSpace(mobile)
	mobile = strings.TrimPrefix(mobile, "+86")
	return mobile, nil
}

// ResolveOrCreateOneTapUser finds the user that owns the verified phone in the
// application's organization, creating one when none exists so that a first-
// time number still logs straight in — the point of one-tap. The created user
// follows the existing u<phone>_<idPrefix> naming so one-tap and app-side
// signups stay uniform. The phone is already carrier-verified here, so no
// SMS code is involved.
func ResolveOrCreateOneTapUser(application *Application, mobile string, countryCode string, lang string) (*User, error) {
	if countryCode == "" {
		countryCode = "CN"
	}

	organizationName := application.Organization

	e164, ok := util.GetE164Number(mobile, countryCode)
	if !ok {
		return nil, fmt.Errorf(i18n.Translate(lang, "verification:Phone number is invalid in your region %s"), countryCode)
	}

	user, err := GetUserByPhone(organizationName, e164)
	if err != nil {
		return nil, err
	}
	if user != nil {
		return user, nil
	}

	organization, err := GetOrganization(util.GetId("admin", organizationName))
	if err != nil {
		return nil, err
	}
	if organization == nil {
		return nil, fmt.Errorf(i18n.Translate(lang, "auth:The organization: %s does not exist"), organizationName)
	}

	initScore, err := organization.GetInitScore()
	if err != nil {
		return nil, err
	}

	id, err := GenerateIdForNewUser(application)
	if err != nil {
		return nil, err
	}

	idPrefix := strings.ReplaceAll(id, "-", "")
	if len(idPrefix) > 8 {
		idPrefix = idPrefix[:8]
	}

	user = &User{
		Owner:             organizationName,
		Name:              fmt.Sprintf("u%s_%s", mobile, idPrefix),
		CreatedTime:       util.GetCurrentTime(),
		Id:                id,
		Type:              "normal-user",
		DisplayName:       mobile,
		Avatar:            organization.DefaultAvatar,
		Phone:             mobile,
		CountryCode:       countryCode,
		Address:           []string{},
		Score:             initScore,
		SignupApplication: application.Name,
		Properties:        map[string]string{},
		RegisterType:      "OneTap Signup",
	}

	affected, err := AddUser(user, lang)
	if err != nil {
		return nil, err
	}
	if !affected {
		return nil, errors.New(i18n.Translate(lang, "general:Failed to add user"))
	}

	return user, nil
}
