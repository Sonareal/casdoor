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
import {Button, Input, Modal, Table} from "antd";
import * as Setting from "./Setting";
import * as ReferralBackend from "./backend/ReferralBackend";
import i18next from "i18next";
import BaseListPage from "./BaseListPage";
import PopconfirmModal from "./common/modal/PopconfirmModal";

class WithdrawalListPage extends BaseListPage {
  constructor(props) {
    super(props);
    this.state = Object.assign(this.state, {
      payModalVisible: false,
      payRecord: null,
      transferNo: "",
    });
  }

  review(record, action) {
    ReferralBackend.reviewWithdrawal(`${record.owner}/${record.name}`, action)
      .then((res) => {
        if (res.status === "ok") {
          Setting.showMessage("success", i18next.t("general:Successfully saved"));
          this.fetch({pagination: this.state.pagination});
        } else {
          Setting.showMessage("error", `${i18next.t("general:Failed to save")}: ${res.msg}`);
        }
      });
  }

  markPaid() {
    const record = this.state.payRecord;
    ReferralBackend.markWithdrawalPaid(`${record.owner}/${record.name}`, "paid", this.state.transferNo)
      .then((res) => {
        if (res.status === "ok") {
          Setting.showMessage("success", i18next.t("general:Successfully saved"));
          this.setState({payModalVisible: false, payRecord: null, transferNo: ""});
          this.fetch({pagination: this.state.pagination});
        } else {
          Setting.showMessage("error", `${i18next.t("general:Failed to save")}: ${res.msg}`);
        }
      });
  }

  markFailed(record) {
    ReferralBackend.markWithdrawalPaid(`${record.owner}/${record.name}`, "fail", "", i18next.t("withdrawal:Transfer failed"))
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
    case "Requested":
      return Setting.getTag("processing", text);
    case "Approved":
      return Setting.getTag("warning", text);
    case "Paid":
      return Setting.getTag("success", text);
    case "Rejected":
      return Setting.getTag("error", text);
    case "Failed":
      return Setting.getTag("error", text);
    default:
      return text;
    }
  }

  renderTable(withdrawals) {
    const columns = [
      {
        title: i18next.t("general:Organization"),
        dataIndex: "owner",
        key: "owner",
        width: "120px",
        sorter: true,
        ...this.getColumnSearchProps("owner"),
      },
      {
        title: i18next.t("general:User"),
        dataIndex: "user",
        key: "user",
        width: "140px",
        sorter: true,
        ...this.getColumnSearchProps("user"),
      },
      {
        title: i18next.t("withdrawal:Amount"),
        dataIndex: "amount",
        key: "amount",
        width: "110px",
        sorter: true,
        render: (text, record, index) => {
          return `${text} ${record.currency}`;
        },
      },
      {
        title: i18next.t("withdrawal:Channel"),
        dataIndex: "channel",
        key: "channel",
        width: "130px",
        sorter: true,
      },
      {
        title: i18next.t("withdrawal:Payee"),
        dataIndex: "payeeName",
        key: "payeeName",
        width: "140px",
        render: (text, record, index) => {
          return `${text} ${record.payeeAccount ? "(" + record.payeeAccount + ")" : ""}`;
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
        title: i18next.t("withdrawal:Transfer No."),
        dataIndex: "externalTransferNo",
        key: "externalTransferNo",
        width: "150px",
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
              {record.state === "Requested" ? (
                <React.Fragment>
                  <PopconfirmModal
                    text={i18next.t("withdrawal:Approve")}
                    title={i18next.t("withdrawal:Approve this withdrawal") + ` (${record.amount} ${record.currency}) ?`}
                    onConfirm={() => this.review(record, "approve")}
                  />
                  <PopconfirmModal
                    text={i18next.t("withdrawal:Reject")}
                    title={i18next.t("withdrawal:Reject this withdrawal") + " ?"}
                    onConfirm={() => this.review(record, "reject")}
                  />
                </React.Fragment>
              ) : null}
              {record.state === "Approved" ? (
                <React.Fragment>
                  <Button style={{marginRight: "10px"}} type="primary" onClick={() => this.setState({payModalVisible: true, payRecord: record, transferNo: ""})}>{i18next.t("withdrawal:Mark paid")}</Button>
                  <PopconfirmModal
                    text={i18next.t("withdrawal:Mark failed")}
                    title={i18next.t("withdrawal:Mark this withdrawal failed") + " ?"}
                    onConfirm={() => this.markFailed(record)}
                  />
                </React.Fragment>
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
        <Table scroll={{x: "max-content"}} columns={columns} dataSource={withdrawals} rowKey={(record) => `${record.owner}/${record.name}`} size="middle" bordered pagination={paginationProps}
          title={() => (
            <div>
              {i18next.t("withdrawal:Withdrawals")}
            </div>
          )}
          loading={this.getTableLoading()}
          onChange={this.handleTableChange}
        />
        <Modal
          title={i18next.t("withdrawal:Mark paid")}
          open={this.state.payModalVisible}
          onOk={() => this.markPaid()}
          onCancel={() => this.setState({payModalVisible: false, payRecord: null, transferNo: ""})}
          okButtonProps={{disabled: this.state.transferNo === ""}}
        >
          <div style={{marginBottom: "10px"}}>{i18next.t("withdrawal:Enter the external transfer No.")}</div>
          <Input value={this.state.transferNo} onChange={(e) => this.setState({transferNo: e.target.value})} />
        </Modal>
      </div>
    );
  }

  fetch = (params = {}) => {
    const field = params.searchedColumn, value = params.searchText;
    const sortField = params.sortField, sortOrder = params.sortOrder;
    this.setState({loading: true});
    ReferralBackend.getWithdrawals(Setting.isDefaultOrganizationSelected(this.props.account) ? "" : Setting.getRequestOrganization(this.props.account), params.pagination.current, params.pagination.pageSize, field, value, sortField, sortOrder)
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

export default WithdrawalListPage;
