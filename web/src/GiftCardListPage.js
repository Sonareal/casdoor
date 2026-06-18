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
import {Button, Input, InputNumber, Modal, Select, Table} from "antd";
import QRCode from "qrcode.react";
import * as Setting from "./Setting";
import * as GiftCardBackend from "./backend/GiftCardBackend";
import * as PlanBackend from "./backend/PlanBackend";
import i18next from "i18next";
import BaseListPage from "./BaseListPage";
import PopconfirmModal from "./common/modal/PopconfirmModal";

class GiftCardListPage extends BaseListPage {
  constructor(props) {
    super(props);
    this.state = Object.assign(this.state, {
      genModalVisible: false,
      genPlan: "",
      genQuantity: 100,
      genBatch: "",
      plans: [],
      qrModalVisible: false,
      qrCode: "",
    });
  }

  openGenerateModal() {
    const owner = Setting.getRequestOrganization(this.props.account);
    PlanBackend.getPlans(owner)
      .then((res) => {
        const plans = Array.isArray(res) ? res : (res.data || []);
        this.setState({
          plans: plans,
          genPlan: plans.length > 0 ? plans[0].name : "",
          genModalVisible: true,
          genBatch: `batch_${Setting.getRandomName ? Setting.getRandomName() : Date.now()}`,
        });
      });
  }

  generate() {
    const owner = Setting.getRequestOrganization(this.props.account);
    const plan = this.state.plans.find((p) => p.name === this.state.genPlan);
    GiftCardBackend.generateGiftCards({
      owner: owner,
      batch: this.state.genBatch,
      plan: this.state.genPlan,
      pricing: "",
      product: plan ? plan.product : "",
      quantity: this.state.genQuantity,
    }).then((res) => {
      if (res.status === "ok") {
        Setting.showMessage("success", `${i18next.t("general:Successfully added")}: ${res.data2}`);
        this.setState({genModalVisible: false});
        this.fetch({pagination: this.state.pagination});
      } else {
        Setting.showMessage("error", `${i18next.t("general:Failed to add")}: ${res.msg}`);
      }
    });
  }

  disable(record) {
    GiftCardBackend.updateGiftCard(record.owner, record.name, Object.assign({}, record, {state: "Disabled"}))
      .then((res) => {
        if (res.status === "ok") {
          Setting.showMessage("success", i18next.t("general:Successfully saved"));
          this.fetch({pagination: this.state.pagination});
        } else {
          Setting.showMessage("error", `${i18next.t("general:Failed to save")}: ${res.msg}`);
        }
      });
  }

  renderStateTag(text) {
    switch (text) {
    case "Unused":
      return Setting.getTag("success", text);
    case "Used":
      return Setting.getTag("default", text);
    case "Disabled":
      return Setting.getTag("error", text);
    default:
      return text;
    }
  }

  renderTable(giftCards) {
    const columns = [
      {
        title: i18next.t("gift:Batch"),
        dataIndex: "batch",
        key: "batch",
        width: "150px",
        sorter: true,
        ...this.getColumnSearchProps("batch"),
      },
      {
        title: i18next.t("gift:Code"),
        dataIndex: "code",
        key: "code",
        width: "300px",
        ...this.getColumnSearchProps("code"),
      },
      {
        title: i18next.t("gift:Plan"),
        dataIndex: "plan",
        key: "plan",
        width: "160px",
        sorter: true,
        ...this.getColumnSearchProps("plan"),
      },
      {
        title: i18next.t("general:State"),
        dataIndex: "state",
        key: "state",
        width: "110px",
        sorter: true,
        ...this.getColumnSearchProps("state"),
        render: (text, record, index) => {
          return this.renderStateTag(text);
        },
      },
      {
        title: i18next.t("gift:Used by"),
        dataIndex: "usedBy",
        key: "usedBy",
        width: "150px",
        ...this.getColumnSearchProps("usedBy"),
      },
      {
        title: i18next.t("general:Created time"),
        dataIndex: "createdTime",
        key: "createdTime",
        width: "160px",
        sorter: true,
        render: (text, record, index) => {
          return Setting.getFormattedDate(text);
        },
      },
      {
        title: i18next.t("general:Action"),
        dataIndex: "",
        key: "op",
        width: "240px",
        fixed: (Setting.isMobile()) ? "false" : "right",
        render: (text, record, index) => {
          return (
            <div>
              <Button style={{marginRight: "10px"}} onClick={() => this.setState({qrModalVisible: true, qrCode: record.code})}>{i18next.t("gift:QR code")}</Button>
              {record.state === "Unused" ? (
                <PopconfirmModal
                  text={i18next.t("gift:Disable")}
                  title={i18next.t("gift:Disable this gift card") + " ?"}
                  onConfirm={() => this.disable(record)}
                />
              ) : null}
            </div>
          );
        },
      },
    ];

    const paginationProps = {
      total: this.state.pagination.total,
      showQuickJumper: true,
      showSizeChanger: true,
      showTotal: () => i18next.t("general:{total} in total").replace("{total}", this.state.pagination.total),
    };

    return (
      <div>
        <Table scroll={{x: "max-content"}} columns={columns} dataSource={giftCards} rowKey={(record) => `${record.owner}/${record.name}`} size="middle" bordered pagination={paginationProps}
          title={() => (
            <div>
              {i18next.t("gift:Gift cards")}&nbsp;&nbsp;&nbsp;&nbsp;
              <Button type="primary" size="small" onClick={() => this.openGenerateModal()}>{i18next.t("gift:Generate")}</Button>
            </div>
          )}
          loading={this.getTableLoading()}
          onChange={this.handleTableChange}
        />
        <Modal
          title={i18next.t("gift:Generate gift cards")}
          open={this.state.genModalVisible}
          onOk={() => this.generate()}
          onCancel={() => this.setState({genModalVisible: false})}
          okButtonProps={{disabled: this.state.genPlan === "" || !(this.state.genQuantity > 0)}}
        >
          <div style={{marginBottom: "8px"}}>{i18next.t("gift:Plan")}:</div>
          <Select style={{width: "100%", marginBottom: "12px"}} value={this.state.genPlan} onChange={(value) => this.setState({genPlan: value})}
            options={this.state.plans.map((p) => ({value: p.name, label: `${p.displayName || p.name} (${p.period})`}))}
          />
          <div style={{marginBottom: "8px"}}>{i18next.t("gift:Quantity")}:</div>
          <InputNumber style={{width: "100%", marginBottom: "12px"}} min={1} max={10000} value={this.state.genQuantity} onChange={(value) => this.setState({genQuantity: value})} />
          <div style={{marginBottom: "8px"}}>{i18next.t("gift:Batch")}:</div>
          <Input value={this.state.genBatch} onChange={(e) => this.setState({genBatch: e.target.value})} />
        </Modal>
        <Modal
          title={i18next.t("gift:QR code")}
          open={this.state.qrModalVisible}
          footer={null}
          onCancel={() => this.setState({qrModalVisible: false})}
        >
          <div style={{textAlign: "center"}}>
            {this.state.qrCode !== "" ? <QRCode value={this.state.qrCode} size={220} /> : null}
            <div style={{marginTop: "12px", wordBreak: "break-all", fontFamily: "monospace"}}>{this.state.qrCode}</div>
          </div>
        </Modal>
      </div>
    );
  }

  fetch = (params = {}) => {
    const field = params.searchedColumn, value = params.searchText;
    const sortField = params.sortField, sortOrder = params.sortOrder;
    this.setState({loading: true});
    GiftCardBackend.getGiftCards(Setting.isDefaultOrganizationSelected(this.props.account) ? "" : Setting.getRequestOrganization(this.props.account), params.pagination.current, params.pagination.pageSize, field, value, sortField, sortOrder)
      .then((res) => {
        this.setState({
          loading: false,
        });
        if (res.status === "ok") {
          this.setState({
            data: res.data,
            pagination: {
              ...params.pagination,
              total: res.data2,
            },
            searchText: params.searchText,
            searchedColumn: params.searchedColumn,
          });
        } else {
          if (Setting.isResponseDenied(res)) {
            this.setState({
              isAuthorized: false,
            });
          } else {
            Setting.showMessage("error", res.msg);
          }
        }
      });
  };
}

export default GiftCardListPage;
