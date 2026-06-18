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
import {Button, Card, Col, Input, InputNumber, Row, Select, Switch, Table} from "antd";
import {DeleteOutlined} from "@ant-design/icons";
import * as Setting from "./Setting";
import * as ReferralBackend from "./backend/ReferralBackend";
import i18next from "i18next";

class ReferralPolicyPage extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      owner: Setting.getRequestOrganization(props.account),
      policy: null,
      groupRows: [],
    };
  }

  componentDidMount() {
    this.getPolicy();
  }

  getPolicy() {
    ReferralBackend.getReferralPolicy(this.state.owner)
      .then((res) => {
        if (res.status === "ok") {
          const policy = res.data;
          const groupRates = policy.groupRates || {};
          const groupRows = Object.keys(groupRates).map((tier) => ({tier: tier, rate: groupRates[tier]}));
          this.setState({policy: policy, groupRows: groupRows});
        } else {
          Setting.showMessage("error", res.msg);
        }
      });
  }

  updateField(field, value) {
    const policy = Setting.deepCopy(this.state.policy);
    policy[field] = value;
    this.setState({policy: policy});
  }

  updateTier(index, field, value) {
    const policy = Setting.deepCopy(this.state.policy);
    const tiers = policy.tiers || [];
    tiers[index][field] = value;
    policy.tiers = tiers;
    this.setState({policy: policy});
  }

  addTier() {
    const policy = Setting.deepCopy(this.state.policy);
    const tiers = policy.tiers || [];
    tiers.push({name: "", minInvites: 0, rate: 0});
    policy.tiers = tiers;
    this.setState({policy: policy});
  }

  deleteTier(index) {
    const policy = Setting.deepCopy(this.state.policy);
    const tiers = policy.tiers || [];
    tiers.splice(index, 1);
    policy.tiers = tiers;
    this.setState({policy: policy});
  }

  updateGroupRow(index, field, value) {
    const groupRows = Setting.deepCopy(this.state.groupRows);
    groupRows[index][field] = value;
    this.setState({groupRows: groupRows});
  }

  addGroupRow() {
    const groupRows = Setting.deepCopy(this.state.groupRows);
    groupRows.push({tier: "", rate: 0});
    this.setState({groupRows: groupRows});
  }

  deleteGroupRow(index) {
    const groupRows = Setting.deepCopy(this.state.groupRows);
    groupRows.splice(index, 1);
    this.setState({groupRows: groupRows});
  }

  submit() {
    const policy = Setting.deepCopy(this.state.policy);
    const groupRates = {};
    this.state.groupRows.forEach((row) => {
      if (row.tier !== "") {
        groupRates[row.tier] = row.rate;
      }
    });
    policy.groupRates = groupRates;
    ReferralBackend.updateReferralPolicy(this.state.owner, policy)
      .then((res) => {
        if (res.status === "ok") {
          Setting.showMessage("success", i18next.t("general:Successfully saved"));
          this.getPolicy();
        } else {
          Setting.showMessage("error", `${i18next.t("general:Failed to save")}: ${res.msg}`);
        }
      });
  }

  renderTiers() {
    const policy = this.state.policy;
    const columns = [
      {
        title: i18next.t("general:Name"),
        dataIndex: "name",
        render: (text, record, index) => {
          return <Input value={text} onChange={(e) => this.updateTier(index, "name", e.target.value)} />;
        },
      },
      {
        title: i18next.t("referral:Min invites"),
        dataIndex: "minInvites",
        render: (text, record, index) => {
          return <InputNumber min={0} value={text} onChange={(value) => this.updateTier(index, "minInvites", value)} />;
        },
      },
      {
        title: i18next.t("referral:Rate"),
        dataIndex: "rate",
        render: (text, record, index) => {
          return <InputNumber min={0} max={1} step={0.01} value={text} onChange={(value) => this.updateTier(index, "rate", value)} />;
        },
      },
      {
        title: i18next.t("general:Action"),
        dataIndex: "op",
        width: "80px",
        render: (text, record, index) => {
          return <Button icon={<DeleteOutlined />} danger onClick={() => this.deleteTier(index)} />;
        },
      },
    ];
    return (
      <Table rowKey={(record, index) => index} columns={columns} dataSource={policy.tiers || []} size="small" pagination={false}
        title={() => (
          <div>
            {i18next.t("referral:Auto-upgrade tiers")}&nbsp;&nbsp;
            <Button type="primary" size="small" onClick={() => this.addTier()}>{i18next.t("general:Add")}</Button>
          </div>
        )}
      />
    );
  }

  renderGroups() {
    const columns = [
      {
        title: i18next.t("referral:Tier"),
        dataIndex: "tier",
        render: (text, record, index) => {
          return <Input value={text} onChange={(e) => this.updateGroupRow(index, "tier", e.target.value)} />;
        },
      },
      {
        title: i18next.t("referral:Rate"),
        dataIndex: "rate",
        render: (text, record, index) => {
          return <InputNumber min={0} max={1} step={0.01} value={text} onChange={(value) => this.updateGroupRow(index, "rate", value)} />;
        },
      },
      {
        title: i18next.t("general:Action"),
        dataIndex: "op",
        width: "80px",
        render: (text, record, index) => {
          return <Button icon={<DeleteOutlined />} danger onClick={() => this.deleteGroupRow(index)} />;
        },
      },
    ];
    return (
      <Table rowKey={(record, index) => index} columns={columns} dataSource={this.state.groupRows} size="small" pagination={false}
        title={() => (
          <div>
            {i18next.t("referral:Group rates")}&nbsp;&nbsp;
            <Button type="primary" size="small" onClick={() => this.addGroupRow()}>{i18next.t("general:Add")}</Button>
          </div>
        )}
      />
    );
  }

  render() {
    const policy = this.state.policy;
    if (policy === null) {
      return null;
    }
    return (
      <div>
        <Card size="small" title={
          <div>
            {i18next.t("referral:Referral policy")}&nbsp;&nbsp;&nbsp;&nbsp;
            <Button type="primary" onClick={() => this.submit()}>{i18next.t("general:Save")}</Button>
          </div>
        } style={{marginLeft: "5px"}}>
          <Row style={{marginTop: "10px"}}>
            <Col style={{marginTop: "5px"}} span={4}>{i18next.t("referral:Enabled")} :</Col>
            <Col span={20}>
              <Switch checked={policy.enabled} onChange={(checked) => this.updateField("enabled", checked)} />
            </Col>
          </Row>
          <Row style={{marginTop: "20px"}}>
            <Col style={{marginTop: "5px"}} span={4}>{i18next.t("referral:Default rate")} :</Col>
            <Col span={20}>
              <InputNumber min={0} max={1} step={0.01} value={policy.defaultRate} onChange={(value) => this.updateField("defaultRate", value)} />
            </Col>
          </Row>
          <Row style={{marginTop: "20px"}}>
            <Col style={{marginTop: "5px"}} span={4}>{i18next.t("referral:Max rate")} :</Col>
            <Col span={20}>
              <InputNumber min={0} max={1} step={0.01} value={policy.maxRate} onChange={(value) => this.updateField("maxRate", value)} />
            </Col>
          </Row>
          <Row style={{marginTop: "20px"}}>
            <Col style={{marginTop: "5px"}} span={4}>{i18next.t("referral:Auto-upgrade")} :</Col>
            <Col span={20}>
              <Switch checked={policy.autoUpgradeEnabled} onChange={(checked) => this.updateField("autoUpgradeEnabled", checked)} />
            </Col>
          </Row>
          <Row style={{marginTop: "20px"}}>
            <Col style={{marginTop: "5px"}} span={4}>{i18next.t("referral:Upgrade basis")} :</Col>
            <Col span={20}>
              <Select style={{width: "200px"}} value={policy.upgradeBasis || "paid"} onChange={(value) => this.updateField("upgradeBasis", value)}
                options={[{value: "paid", label: i18next.t("referral:paid (first orders)")}, {value: "signup", label: i18next.t("referral:signup (registrations)")}]}
              />
            </Col>
          </Row>
          <Row style={{marginTop: "20px"}}>
            <Col span={24}>{this.renderTiers()}</Col>
          </Row>
          <Row style={{marginTop: "20px"}}>
            <Col span={24}>{this.renderGroups()}</Col>
          </Row>
        </Card>
      </div>
    );
  }
}

export default ReferralPolicyPage;
