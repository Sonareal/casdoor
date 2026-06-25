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
import {Table, Tooltip} from "antd";
import * as Setting from "./Setting";
import * as EventBackend from "./backend/EventBackend";
import i18next from "i18next";
import BaseListPage from "./BaseListPage";

class EventListPage extends BaseListPage {
  renderTable(events) {
    const columns = [
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
        title: i18next.t("analytics:Event"),
        dataIndex: "event",
        key: "event",
        width: "180px",
        sorter: true,
        ...this.getColumnSearchProps("event"),
      },
      {
        title: i18next.t("analytics:Source"),
        dataIndex: "source",
        key: "source",
        width: "100px",
        ...this.getColumnSearchProps("source"),
        render: (text, record, index) => {
          return text === "server" ? Setting.getTag("blue", text) : Setting.getTag("default", text);
        },
      },
      {
        title: i18next.t("general:User"),
        dataIndex: "user",
        key: "user",
        width: "150px",
        ...this.getColumnSearchProps("user"),
        render: (text, record, index) => {
          if (!text) {
            return record.distinctId ? <span style={{color: "#999"}}>{record.distinctId}</span> : null;
          }
          return <Link to={`/users/${text}`}>{text}</Link>;
        },
      },
      {
        title: i18next.t("analytics:Platform"),
        dataIndex: "platform",
        key: "platform",
        width: "110px",
        ...this.getColumnSearchProps("platform"),
      },
      {
        title: i18next.t("general:Application"),
        dataIndex: "application",
        key: "application",
        width: "130px",
        ...this.getColumnSearchProps("application"),
      },
      {
        title: i18next.t("analytics:Properties"),
        dataIndex: "properties",
        key: "properties",
        render: (text, record, index) => {
          if (!text) {
            return null;
          }
          return (
            <Tooltip title={text}>
              <span style={{fontFamily: "monospace", maxWidth: "400px", display: "inline-block", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", verticalAlign: "middle"}}>{text}</span>
            </Tooltip>
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
        <Table scroll={{x: "max-content"}} columns={columns} dataSource={events} rowKey={(record) => `${record.owner}/${record.name}`} size="middle" bordered pagination={paginationProps}
          title={() => (
            <div>
              {i18next.t("analytics:Events")}
            </div>
          )}
          loading={this.getTableLoading()}
          onChange={this.handleTableChange}
        />
      </div>
    );
  }

  fetch = (params = {}) => {
    const field = params.searchedColumn, value = params.searchText;
    const sortField = params.sortField, sortOrder = params.sortOrder;
    this.setState({loading: true});
    EventBackend.getEvents(Setting.isDefaultOrganizationSelected(this.props.account) ? "" : Setting.getRequestOrganization(this.props.account), params.pagination.current, params.pagination.pageSize, field, value, sortField, sortOrder)
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

export default EventListPage;
