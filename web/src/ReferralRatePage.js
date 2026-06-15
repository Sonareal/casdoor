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

import React from "react";
import {Button, Card, Col, Input, InputNumber, Row, Tag} from "antd";
import * as Setting from "./Setting";
import * as ReferralBackend from "./backend/ReferralBackend";
import i18next from "i18next";

class ReferralRatePage extends React.Component {
  constructor(props) {
    super(props);
    const owner = Setting.getRequestOrganization(props.account);
    this.state = {
      userId: `${owner}/`,
      info: null,
      rate: -1,
      tier: "",
    };
  }

  load() {
    if (this.state.userId === "" || this.state.userId.endsWith("/")) {
      Setting.showMessage("error", i18next.t("referral:Please enter a user id (org/name)"));
      return;
    }
    ReferralBackend.getReferralRate(this.state.userId)
      .then((res) => {
        if (res.status === "ok") {
          this.setState({info: res.data, rate: res.data.commissionRate, tier: res.data.tier || ""});
        } else {
          Setting.showMessage("error", res.msg);
        }
      });
  }

  save() {
    ReferralBackend.setReferralRate({user: this.state.userId, rate: this.state.rate, tier: this.state.tier})
      .then((res) => {
        if (res.status === "ok") {
          Setting.showMessage("success", i18next.t("general:Successfully saved"));
          this.load();
        } else {
          Setting.showMessage("error", `${i18next.t("general:Failed to save")}: ${res.msg}`);
        }
      });
  }

  render() {
    const info = this.state.info;
    return (
      <div>
        <Card size="small" title={i18next.t("referral:Per-user rate")} style={{marginLeft: "5px"}}>
          <Row style={{marginTop: "10px"}}>
            <Col style={{marginTop: "5px"}} span={4}>{i18next.t("general:User")} :</Col>
            <Col span={20}>
              <Input style={{width: "300px", marginRight: "10px"}} value={this.state.userId} onChange={(e) => this.setState({userId: e.target.value})} placeholder="organization/username" />
              <Button type="primary" onClick={() => this.load()}>{i18next.t("referral:Load")}</Button>
            </Col>
          </Row>
          {info === null ? null : (
            <React.Fragment>
              <Row style={{marginTop: "20px"}}>
                <Col style={{marginTop: "5px"}} span={4}>{i18next.t("referral:Effective rate")} :</Col>
                <Col span={20} style={{marginTop: "5px"}}>
                  <Tag color="blue">{info.effectiveRate}</Tag>
                  <Tag>{info.rateSource}</Tag>
                  {i18next.t("referral:Paid invites")}: {info.paidInviteCount}
                </Col>
              </Row>
              <Row style={{marginTop: "20px"}}>
                <Col style={{marginTop: "5px"}} span={4}>{i18next.t("referral:Override rate")} :</Col>
                <Col span={20}>
                  <InputNumber min={-1} max={1} step={0.01} value={this.state.rate} onChange={(value) => this.setState({rate: value})} />
                  <span style={{marginLeft: "10px", color: "#999"}}>{i18next.t("referral:-1 = unset, 0 = explicit 0%")}</span>
                </Col>
              </Row>
              <Row style={{marginTop: "20px"}}>
                <Col style={{marginTop: "5px"}} span={4}>{i18next.t("referral:Tier")} :</Col>
                <Col span={20}>
                  <Input style={{width: "200px"}} value={this.state.tier} onChange={(e) => this.setState({tier: e.target.value})} placeholder="vip" />
                </Col>
              </Row>
              <Row style={{marginTop: "20px"}}>
                <Col span={4}></Col>
                <Col span={20}>
                  <Button type="primary" onClick={() => this.save()}>{i18next.t("general:Save")}</Button>
                </Col>
              </Row>
            </React.Fragment>
          )}
        </Card>
      </div>
    );
  }
}

export default ReferralRatePage;
