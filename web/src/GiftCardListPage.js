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
import {Link} from "react-router-dom";
import {Button, Input, InputNumber, Modal, Popconfirm, Select, Space, Table} from "antd";
import {QRCodeCanvas} from "qrcode.react";
import copy from "copy-to-clipboard";
import {saveAs} from "file-saver";
import JSZip from "jszip";
import * as Setting from "./Setting";
import * as GiftCardBackend from "./backend/GiftCardBackend";
import * as PlanBackend from "./backend/PlanBackend";
import i18next from "i18next";
import BaseListPage from "./BaseListPage";

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
      selectedRowKeys: [],
      selectedRows: [],
      batchValue: "",
      qrExportCodes: [],
    });
  }

  getOwner() {
    return Setting.getRequestOrganization(this.props.account);
  }

  openGenerateModal() {
    PlanBackend.getPlans(this.getOwner())
      .then((res) => {
        const plans = Array.isArray(res) ? res : (res.data || []);
        this.setState({
          plans: plans,
          genPlan: plans.length > 0 ? plans[0].name : "",
          genModalVisible: true,
          genBatch: `batch_${Setting.getRandomName()}`,
        });
      });
  }

  generate() {
    const plan = this.state.plans.find((p) => p.name === this.state.genPlan);
    GiftCardBackend.generateGiftCards({
      owner: this.getOwner(),
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

  copyCode(code) {
    copy(code);
    Setting.showMessage("success", i18next.t("gift:Copied"));
  }

  copyCodes(codes) {
    if (codes.length === 0) {
      Setting.showMessage("error", i18next.t("gift:Nothing selected"));
      return;
    }
    copy(codes.join("\n"));
    Setting.showMessage("success", `${i18next.t("gift:Copied")}: ${codes.length}`);
  }

  downloadSinglePng() {
    const canvas = document.getElementById("gc-qr-single");
    if (canvas) {
      canvas.toBlob((blob) => saveAs(blob, `${this.state.qrCode}.png`));
    }
  }

  // render hidden QR canvases for the given codes, then zip them into a single download
  exportZip(codes) {
    if (codes.length === 0) {
      Setting.showMessage("error", i18next.t("gift:Nothing selected"));
      return;
    }
    this.setState({qrExportCodes: codes}, () => {
      setTimeout(() => {
        const zip = new JSZip();
        let pending = codes.length;
        codes.forEach((code) => {
          const canvas = document.getElementById(`gc-qrexp-${code}`);
          if (!canvas) {
            pending--;
            return;
          }
          canvas.toBlob((blob) => {
            zip.file(`${code}.png`, blob);
            pending--;
            if (pending === 0) {
              zip.generateAsync({type: "blob"}).then((content) => {
                saveAs(content, "gift-cards-qr.zip");
                this.setState({qrExportCodes: []});
              });
            }
          });
        });
      }, 400);
    });
  }

  batchAction(action, names, batch) {
    GiftCardBackend.batchGiftCards({owner: this.getOwner(), action: action, names: names, batch: batch})
      .then((res) => {
        if (res.status === "ok") {
          Setting.showMessage("success", `${i18next.t("general:Successfully saved")}: ${res.data}`);
          this.setState({selectedRowKeys: [], selectedRows: []});
          this.fetch({pagination: this.state.pagination});
        } else {
          Setting.showMessage("error", `${i18next.t("general:Failed to save")}: ${res.msg}`);
        }
      });
  }

  // fetch all codes of a batch (across pages) for copy / zip
  withBatchCodes(batch, callback) {
    if (batch === "") {
      Setting.showMessage("error", i18next.t("gift:Select a batch"));
      return;
    }
    GiftCardBackend.getGiftCards(this.getOwner(), 1, 100000, "batch", batch)
      .then((res) => {
        if (res.status === "ok") {
          callback((res.data || []).map((gc) => gc.code));
        } else {
          Setting.showMessage("error", res.msg);
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

  renderToolbar() {
    const names = this.state.selectedRows.map((r) => r.name);
    const codes = this.state.selectedRows.map((r) => r.code);
    const hasSel = names.length > 0;
    const batches = Array.from(new Set((this.state.data || []).map((r) => r.batch).filter((b) => b !== "")));

    return (
      <div style={{marginBottom: "10px"}}>
        <Space wrap>
          <span>{i18next.t("gift:Selected")}: {names.length}</span>
          <Button disabled={!hasSel} onClick={() => this.copyCodes(codes)}>{i18next.t("gift:Copy codes")}</Button>
          <Button disabled={!hasSel} onClick={() => this.exportZip(codes)}>{i18next.t("gift:Download QR (zip)")}</Button>
          <Popconfirm disabled={!hasSel} title={i18next.t("gift:Disable selected") + " ?"} onConfirm={() => this.batchAction("disable", names, "")} okText={i18next.t("general:OK")} cancelText={i18next.t("general:Cancel")}>
            <Button disabled={!hasSel}>{i18next.t("gift:Disable selected")}</Button>
          </Popconfirm>
          <Popconfirm disabled={!hasSel} title={i18next.t("gift:Delete selected") + " ?"} onConfirm={() => this.batchAction("delete", names, "")} okText={i18next.t("general:OK")} cancelText={i18next.t("general:Cancel")}>
            <Button danger disabled={!hasSel}>{i18next.t("gift:Delete selected")}</Button>
          </Popconfirm>
        </Space>
        <Space wrap style={{marginLeft: "20px"}}>
          <span>{i18next.t("gift:By batch")}:</span>
          <Select style={{width: "200px"}} value={this.state.batchValue} onChange={(value) => this.setState({batchValue: value})}
            placeholder={i18next.t("gift:Select a batch")} options={batches.map((b) => ({value: b, label: b}))} allowClear
          />
          <Button disabled={this.state.batchValue === ""} onClick={() => this.withBatchCodes(this.state.batchValue, (codes) => this.copyCodes(codes))}>{i18next.t("gift:Copy codes")}</Button>
          <Button disabled={this.state.batchValue === ""} onClick={() => this.withBatchCodes(this.state.batchValue, (codes) => this.exportZip(codes))}>{i18next.t("gift:Download QR (zip)")}</Button>
          <Popconfirm disabled={this.state.batchValue === ""} title={i18next.t("gift:Disable this batch") + " ?"} onConfirm={() => this.batchAction("disable", [], this.state.batchValue)} okText={i18next.t("general:OK")} cancelText={i18next.t("general:Cancel")}>
            <Button disabled={this.state.batchValue === ""}>{i18next.t("gift:Disable selected")}</Button>
          </Popconfirm>
          <Popconfirm disabled={this.state.batchValue === ""} title={i18next.t("gift:Delete this batch") + " ?"} onConfirm={() => this.batchAction("delete", [], this.state.batchValue)} okText={i18next.t("general:OK")} cancelText={i18next.t("general:Cancel")}>
            <Button danger disabled={this.state.batchValue === ""}>{i18next.t("gift:Delete selected")}</Button>
          </Popconfirm>
        </Space>
      </div>
    );
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
        width: "200px",
        ...this.getColumnSearchProps("code"),
        render: (text, record, index) => {
          return <span style={{fontFamily: "monospace"}}>{text}</span>;
        },
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
        render: (text, record, index) => {
          if (!text) {
            return null;
          }
          return (
            <Link to={`/users/${record.owner}/${text}`}>
              {text}
            </Link>
          );
        },
      },
      {
        title: i18next.t("gift:Used time"),
        dataIndex: "usedTime",
        key: "usedTime",
        width: "160px",
        sorter: true,
        render: (text, record, index) => {
          return text ? Setting.getFormattedDate(text) : null;
        },
      },
      {
        title: i18next.t("gift:Expires at"),
        dataIndex: "subEndTime",
        key: "subEndTime",
        width: "160px",
        sorter: true,
        render: (text, record, index) => {
          return text ? Setting.getFormattedDate(text) : null;
        },
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
        width: "260px",
        fixed: (Setting.isMobile()) ? "false" : "right",
        render: (text, record, index) => {
          return (
            <Space>
              <Button onClick={() => this.copyCode(record.code)}>{i18next.t("gift:Copy")}</Button>
              <Button onClick={() => this.setState({qrModalVisible: true, qrCode: record.code})}>{i18next.t("gift:QR code")}</Button>
              {record.state === "Unused" ? (
                <Popconfirm title={i18next.t("gift:Disable this gift card") + " ?"} onConfirm={() => this.batchAction("disable", [record.name], "")} okText={i18next.t("general:OK")} cancelText={i18next.t("general:Cancel")}>
                  <Button>{i18next.t("gift:Disable")}</Button>
                </Popconfirm>
              ) : null}
              <Popconfirm title={i18next.t("general:Sure to delete") + " ?"} onConfirm={() => this.batchAction("delete", [record.name], "")} okText={i18next.t("general:OK")} cancelText={i18next.t("general:Cancel")}>
                <Button danger>{i18next.t("general:Delete")}</Button>
              </Popconfirm>
            </Space>
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

    const rowSelection = {
      selectedRowKeys: this.state.selectedRowKeys,
      onChange: (selectedRowKeys, selectedRows) => {
        this.setState({selectedRowKeys: selectedRowKeys, selectedRows: selectedRows});
      },
    };

    return (
      <div>
        <Table scroll={{x: "max-content"}} columns={columns} dataSource={giftCards} rowKey={(record) => `${record.owner}/${record.name}`} rowSelection={rowSelection} size="middle" bordered pagination={paginationProps}
          title={() => (
            <div>
              {i18next.t("gift:Gift cards")}&nbsp;&nbsp;&nbsp;&nbsp;
              <Button type="primary" size="small" onClick={() => this.openGenerateModal()}>{i18next.t("gift:Generate")}</Button>
              <div style={{marginTop: "10px"}}>{this.renderToolbar()}</div>
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
            {this.state.qrCode !== "" ? <QRCodeCanvas id="gc-qr-single" value={this.state.qrCode} size={220} /> : null}
            <div style={{marginTop: "12px", wordBreak: "break-all", fontFamily: "monospace"}}>{this.state.qrCode}</div>
            <div style={{marginTop: "12px"}}>
              <Button onClick={() => this.copyCode(this.state.qrCode)} style={{marginRight: "10px"}}>{i18next.t("gift:Copy")}</Button>
              <Button type="primary" onClick={() => this.downloadSinglePng()}>{i18next.t("gift:Download PNG")}</Button>
            </div>
          </div>
        </Modal>
        <div style={{display: "none"}}>
          {this.state.qrExportCodes.map((code) => (
            <QRCodeCanvas key={code} id={`gc-qrexp-${code}`} value={code} size={256} />
          ))}
        </div>
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
