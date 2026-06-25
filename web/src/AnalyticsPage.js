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
import {Card, Col, Progress, Row, Select, Statistic, Table} from "antd";
import * as Setting from "./Setting";
import * as EventBackend from "./backend/EventBackend";
import i18next from "i18next";

class AnalyticsPage extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      owner: Setting.getRequestOrganization(props.account),
      days: 14,
      stats: null,
      loading: false,
    };
  }

  componentDidMount() {
    this.getStats();
  }

  getStats() {
    this.setState({loading: true});
    EventBackend.getEventStats(this.state.owner, this.state.days)
      .then((res) => {
        this.setState({loading: false});
        if (res.status === "ok") {
          this.setState({stats: res.data});
        } else {
          Setting.showMessage("error", res.msg);
        }
      });
  }

  renderFunnel(funnel) {
    const base = (funnel && funnel.length > 0 && funnel[0].users > 0) ? funnel[0].users : 0;
    return (
      <Card size="small" title={i18next.t("analytics:Conversion funnel")} style={{marginTop: "16px"}}>
        {(funnel || []).map((step, i) => {
          const pct = base > 0 ? Math.round((step.users / base) * 100) : 0;
          return (
            <div key={step.event} style={{marginBottom: "12px"}}>
              <div style={{display: "flex", justifyContent: "space-between"}}>
                <span>{i + 1}. {i18next.t(`analytics:${step.event}`)} <span style={{color: "#999"}}>({step.event})</span></span>
                <span>{step.users} {i18next.t("analytics:users")} / {step.count} {i18next.t("analytics:events")}</span>
              </div>
              <Progress percent={pct} status="active" />
            </div>
          );
        })}
      </Card>
    );
  }

  render() {
    const stats = this.state.stats;
    const topColumns = [
      {title: i18next.t("analytics:Event"), dataIndex: "key", key: "key"},
      {title: i18next.t("analytics:Count"), dataIndex: "count", key: "count", sorter: (a, b) => a.count - b.count},
      {title: i18next.t("analytics:Users"), dataIndex: "users", key: "users", sorter: (a, b) => a.users - b.users},
    ];
    const dailyColumns = [
      {title: i18next.t("analytics:Date"), dataIndex: "key", key: "key"},
      {title: i18next.t("analytics:Events"), dataIndex: "count", key: "count"},
      {title: i18next.t("analytics:Users"), dataIndex: "users", key: "users"},
    ];

    return (
      <div style={{padding: "10px"}}>
        <div style={{marginBottom: "16px"}}>
          {i18next.t("analytics:Lookback")}:&nbsp;
          <Select value={this.state.days} style={{width: "120px"}} onChange={(v) => this.setState({days: v}, () => this.getStats())}
            options={[{value: 7, label: "7"}, {value: 14, label: "14"}, {value: 30, label: "30"}]}
          />
        </div>
        {stats === null ? null : (
          <div>
            <Row gutter={16}>
              <Col span={8}><Card size="small"><Statistic title={i18next.t("analytics:Events today")} value={stats.totalToday} /></Card></Col>
              <Col span={8}><Card size="small"><Statistic title={i18next.t("analytics:Active users today")} value={stats.dauToday} /></Card></Col>
              <Col span={8}><Card size="small"><Statistic title={i18next.t("analytics:Lookback days")} value={stats.days} /></Card></Col>
            </Row>
            {this.renderFunnel(stats.funnel)}
            <Row gutter={16} style={{marginTop: "16px"}}>
              <Col span={12}>
                <Card size="small" title={i18next.t("analytics:Top events")}>
                  <Table rowKey="key" size="small" pagination={false} columns={topColumns} dataSource={stats.topEvents || []} loading={this.state.loading} />
                </Card>
              </Col>
              <Col span={12}>
                <Card size="small" title={i18next.t("analytics:Daily trend")}>
                  <Table rowKey="key" size="small" pagination={{pageSize: 10}} columns={dailyColumns} dataSource={stats.daily || []} loading={this.state.loading} />
                </Card>
              </Col>
            </Row>
          </div>
        )}
      </div>
    );
  }
}

export default AnalyticsPage;
