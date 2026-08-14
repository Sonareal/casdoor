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
import {DeleteOutlined, DownOutlined, UpOutlined} from "@ant-design/icons";
import {Button, Col, Input, Radio, Row, Switch, Table, Tooltip} from "antd";
import * as Setting from "../Setting";
import i18next from "i18next";

// Declares which custom properties an organization accepts on its users, and
// which single one the back office may search accounts by.
class CustomPropertyTable extends React.Component {
  constructor(props) {
    super(props);
    this.state = {classes: props};
  }

  updateTable(table) {
    this.props.onUpdateTable(table);
  }

  updateField(table, index, key, value) {
    table[index][key] = value;
    this.updateTable(table);
  }

  // Only one property can be the lookup key, so selecting one clears the rest.
  // Enforced here rather than left to the operator: two primaries would make
  // "which key does the back office search by" ambiguous.
  setPrimary(table, index) {
    table.forEach((row, i) => {
      row.isPrimary = i === index;
    });
    this.updateTable(table);
  }

  addRow(table) {
    const row = {name: "", displayName: "", description: "", allowedValues: "", isMultiValue: false, isPrimary: false};
    if (table === undefined) {
      table = [];
    }
    this.updateTable([...table, row]);
  }

  deleteRow(table, i) {
    this.updateTable(Setting.deleteRow(table, i));
  }

  upRow(table, i) {
    this.updateTable(Setting.swapRow(table, i - 1, i));
  }

  downRow(table, i) {
    this.updateTable(Setting.swapRow(table, i, i + 1));
  }

  renderTable(table) {
    const columns = [
      {
        title: i18next.t("general:Name"),
        dataIndex: "name",
        key: "name",
        width: "180px",
        render: (text, record, index) => (
          <Input
            value={text}
            placeholder="deviceId"
            onChange={e => {this.updateField(table, index, "name", e.target.value.trim());}}
          />
        ),
      },
      {
        title: i18next.t("general:Display name"),
        dataIndex: "displayName",
        key: "displayName",
        width: "180px",
        render: (text, record, index) => (
          <Input
            value={text}
            placeholder={i18next.t("organization:Device ID")}
            onChange={e => {this.updateField(table, index, "displayName", e.target.value);}}
          />
        ),
      },
      {
        title: i18next.t("general:Description"),
        dataIndex: "description",
        key: "description",
        render: (text, record, index) => (
          <Input
            value={text}
            onChange={e => {this.updateField(table, index, "description", e.target.value);}}
          />
        ),
      },
      {
        title: (
          <Tooltip placement="top" title={i18next.t("organization:Comma-separated accepted values; leave empty for free text")}>
            {i18next.t("organization:Allowed values")}
          </Tooltip>
        ),
        dataIndex: "allowedValues",
        key: "allowedValues",
        width: "200px",
        render: (text, record, index) => (
          <Input
            value={text}
            placeholder="zh,en"
            onChange={e => {this.updateField(table, index, "allowedValues", e.target.value);}}
          />
        ),
      },
      {
        title: (
          <Tooltip placement="top" title={i18next.t("organization:Store a comma-separated set; writes append and the lookup matches any single item")}>
            {i18next.t("organization:Multi-value")}
          </Tooltip>
        ),
        dataIndex: "isMultiValue",
        key: "isMultiValue",
        width: "100px",
        render: (text, record, index) => (
          <Switch
            checked={!!text}
            onChange={checked => {this.updateField(table, index, "isMultiValue", checked);}}
          />
        ),
      },
      {
        title: (
          <Tooltip placement="top" title={i18next.t("organization:Only the primary property can be searched by the back office lookup API")}>
            {i18next.t("organization:Lookup key")}
          </Tooltip>
        ),
        dataIndex: "isPrimary",
        key: "isPrimary",
        width: "110px",
        render: (text, record, index) => (
          <Radio
            checked={!!text}
            onChange={() => {this.setPrimary(table, index);}}
          />
        ),
      },
      {
        title: i18next.t("general:Action"),
        key: "action",
        width: "100px",
        render: (text, record, index) => (
          <div>
            <Tooltip placement="bottomLeft" title={i18next.t("general:Up")}>
              <Button style={{marginRight: "5px"}} disabled={index === 0} icon={<UpOutlined />} size="small" onClick={() => this.upRow(table, index)} />
            </Tooltip>
            <Tooltip placement="topLeft" title={i18next.t("general:Down")}>
              <Button style={{marginRight: "5px"}} disabled={index === table.length - 1} icon={<DownOutlined />} size="small" onClick={() => this.downRow(table, index)} />
            </Tooltip>
            <Tooltip placement="topLeft" title={i18next.t("general:Delete")}>
              <Button icon={<DeleteOutlined />} size="small" onClick={() => this.deleteRow(table, index)} />
            </Tooltip>
          </div>
        ),
      },
    ];

    return (
      <Table
        scroll={{x: "max-content"}}
        rowKey={(record, index) => index}
        columns={columns}
        dataSource={table}
        size="middle"
        bordered
        pagination={false}
        title={() => (
          <div>
            {this.props.title}&nbsp;&nbsp;&nbsp;&nbsp;
            <Button style={{marginRight: "5px"}} type="primary" size="small" onClick={() => this.addRow(table)}>
              {i18next.t("general:Add")}
            </Button>
          </div>
        )}
      />
    );
  }

  render() {
    return (
      <Col span={24}>
        <Row style={{marginTop: "20px"}}>
          <Col span={24}>
            {this.renderTable(this.props.table)}
          </Col>
        </Row>
      </Col>
    );
  }
}

export default CustomPropertyTable;
