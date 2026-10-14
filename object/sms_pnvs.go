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
	"encoding/json"
	"errors"
	"fmt"

	"github.com/aliyun/alibaba-cloud-sdk-go/services/dypnsapi"
)

type PnvsSmsClient struct {
	template string
	sign     string
	core     *dypnsapi.Client
}

func newPnvsSmsClient(accessId string, accessKey string, sign string, template string, regionId string) (*PnvsSmsClient, error) {
	if regionId == "" {
		regionId = "cn-hangzhou"
	}

	client, err := dypnsapi.NewClientWithAccessKey(regionId, accessId, accessKey)
	if err != nil {
		return nil, err
	}

	pnvsClient := &PnvsSmsClient{
		template: template,
		core:     client,
		sign:     sign,
	}

	return pnvsClient, nil
}

// GetMobile exchanges a carrier one-tap access token (Aliyun spToken, obtained
// by the app's SDK on the device) for the verified phone number. The AccessKey
// pair is what authorises the exchange, which is exactly why it must happen
// here on the server and never in the app.
func (c *PnvsSmsClient) GetMobile(accessToken string) (string, error) {
	if accessToken == "" {
		return "", fmt.Errorf("missing parameter: accessToken")
	}

	request := dypnsapi.CreateGetMobileRequest()
	request.Scheme = "https"
	request.AccessToken = accessToken

	response, err := c.core.GetMobile(request)
	if err != nil {
		return "", err
	}

	if response.Code != "OK" {
		if response.Message != "" {
			return "", errors.New(response.Message)
		}
		return "", fmt.Errorf("PNVS GetMobile failed with code: %s", response.Code)
	}

	mobile := response.GetMobileResultDTO.Mobile
	if mobile == "" {
		return "", fmt.Errorf("PNVS GetMobile returned an empty mobile")
	}

	return mobile, nil
}

func (c *PnvsSmsClient) SendMessage(param map[string]string, targetPhoneNumber ...string) error {
	if len(targetPhoneNumber) == 0 {
		return fmt.Errorf("missing parameter: targetPhoneNumber")
	}

	// PNVS sends to one phone number at a time
	phoneNumber := targetPhoneNumber[0]

	request := dypnsapi.CreateSendSmsVerifyCodeRequest()
	request.Scheme = "https"
	request.PhoneNumber = phoneNumber
	request.TemplateCode = c.template
	request.SignName = c.sign

	// TemplateParam is optional for PNVS as it can auto-generate verification codes
	// But if params are provided, we'll pass them
	if len(param) > 0 {
		templateParam, err := json.Marshal(param)
		if err != nil {
			return err
		}
		request.TemplateParam = string(templateParam)
	}

	response, err := c.core.SendSmsVerifyCode(request)
	if err != nil {
		return err
	}

	if response.Code != "OK" {
		if response.Message != "" {
			return errors.New(response.Message)
		}
		return fmt.Errorf("PNVS SMS send failed with code: %s", response.Code)
	}

	return nil
}
