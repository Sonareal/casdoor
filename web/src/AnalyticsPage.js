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
import {Card, Col, Progress, Row, Select, Statistic, Table, Tag, Tooltip} from "antd";
import * as Setting from "./Setting";
import * as EventBackend from "./backend/EventBackend";
import i18next from "i18next";

class AnalyticsPage extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      owner: this.getOwner(),
      days: 14,
      stats: null,
      loading: false,
    };
    this.handleOrgChange = this.handleOrgChange.bind(this);
  }

  getOwner() {
    return Setting.isDefaultOrganizationSelected(this.props.account) ? "" : Setting.getRequestOrganization(this.props.account);
  }

  componentDidMount() {
    window.addEventListener("storageOrganizationChanged", this.handleOrgChange);
    this.getStats();
  }

  componentWillUnmount() {
    window.removeEventListener("storageOrganizationChanged", this.handleOrgChange);
  }

  handleOrgChange() {
    // follow the top-bar organization selector
    this.setState({owner: this.getOwner()}, () => this.getStats());
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

  metricCard(title, value, opts = {}) {
    return (
      <Col span={opts.span || 6} style={{marginBottom: "16px"}}>
        <Card size="small"><Statistic title={title} value={value} precision={opts.precision} prefix={opts.prefix} suffix={opts.suffix} valueStyle={opts.color ? {color: opts.color} : undefined} /></Card>
      </Col>
    );
  }

  renderFunnel(funnel) {
    const base = (funnel && funnel.length > 0 && funnel[0].users > 0) ? funnel[0].users : 0;
    return (
      <Card size="small" title={i18next.t("analytics:Conversion funnel")} style={{marginTop: "8px"}}>
        {(funnel || []).map((step, i) => {
          const prev = i > 0 ? funnel[i - 1].users : step.users;
          const overall = base > 0 ? Math.round((step.users / base) * 100) : 0;
          const stepRate = (i > 0 && prev > 0) ? Math.round((step.users / prev) * 100) : 100;
          return (
            <div key={step.event} style={{marginBottom: "14px"}}>
              <div style={{display: "flex", justifyContent: "space-between"}}>
                <span>{i + 1}. {i18next.t(`analytics:${step.event}`)} <span style={{color: "#999"}}>({step.event})</span></span>
                <span>
                  <b>{step.users}</b> {i18next.t("analytics:users")}
                  {i > 0 ? <Tag color={stepRate >= 50 ? "green" : (stepRate >= 20 ? "orange" : "red")} style={{marginLeft: "8px"}}>{i18next.t("analytics:Step rate")} {stepRate}%</Tag> : null}
                </span>
              </div>
              <Progress percent={overall} status="active" format={(p) => `${p}%`} />
            </div>
          );
        })}
      </Card>
    );
  }

  renderTrend(daily) {
    const data = daily || [];
    const max = data.reduce((m, d) => Math.max(m, d.count), 1);
    return (
      <Card size="small" title={i18next.t("analytics:Daily trend")} loading={this.state.loading}>
        <div style={{display: "flex", alignItems: "flex-end", height: "140px", gap: "4px", overflowX: "auto", paddingTop: "10px"}}>
          {data.map((d) => (
            <Tooltip key={d.key} title={`${d.key} · ${i18next.t("analytics:Events")}: ${d.count} · ${i18next.t("analytics:Users")}: ${d.users}`}>
              <div style={{display: "flex", flexDirection: "column", alignItems: "center", minWidth: "26px"}}>
                <div style={{fontSize: "11px", color: "#888"}}>{d.count}</div>
                <div style={{width: "18px", height: `${Math.round((d.count / max) * 110)}px`, background: "#1677ff", borderRadius: "3px 3px 0 0"}} />
                <div style={{fontSize: "10px", color: "#aaa", marginTop: "2px", transform: "rotate(-45deg)", transformOrigin: "center", whiteSpace: "nowrap"}}>{d.key.slice(5)}</div>
              </div>
            </Tooltip>
          ))}
          {data.length === 0 ? <span style={{color: "#999"}}>{i18next.t("analytics:No data")}</span> : null}
        </div>
      </Card>
    );
  }

  render() {
    const stats = this.state.stats;
    const m = (stats && stats.metrics) || {};
    const breakdownCols = (titleKey) => [
      {title: i18next.t(titleKey), dataIndex: "key", key: "key", render: (t) => t || <span style={{color: "#999"}}>—</span>},
      {title: i18next.t("analytics:Count"), dataIndex: "count", key: "count", sorter: (a, b) => a.count - b.count},
      {title: i18next.t("analytics:Users"), dataIndex: "users", key: "users", sorter: (a, b) => a.users - b.users},
    ];
    const orgLabel = Setting.getOrganization() === "All" ? i18next.t("general:All") : Setting.getOrganization();

    return (
      <div style={{padding: "10px"}}>
        <div style={{marginBottom: "16px", display: "flex", alignItems: "center", gap: "16px"}}>
          <span>{i18next.t("analytics:Organization")}: <Tag color="blue">{orgLabel}</Tag></span>
          <span>{i18next.t("analytics:Lookback")}:&nbsp;
            <Select value={this.state.days} style={{width: "110px"}} onChange={(v) => this.setState({days: v}, () => this.getStats())}
              options={[{value: 7, label: "7"}, {value: 14, label: "14"}, {value: 30, label: "30"}, {value: 90, label: "90"}]}
            />
          </span>
        </div>
        {stats === null ? null : (
          <div>
            <Row gutter={16}>
              {this.metricCard(i18next.t("analytics:Active users"), m.activeUsers)}
              {this.metricCard(i18next.t("analytics:New users"), m.newUsers, {color: "#1677ff"})}
              {this.metricCard(i18next.t("analytics:Subscriptions"), m.subscriptions, {color: "#52c41a"})}
              {this.metricCard(i18next.t("analytics:Gift redeems"), m.giftRedeems)}
            </Row>
            <Row gutter={16}>
              {this.metricCard(i18next.t("analytics:Payments"), m.payments)}
              {this.metricCard(i18next.t("analytics:Revenue"), m.revenue, {precision: 2, prefix: "¥", color: "#cf1322"})}
              {this.metricCard(i18next.t("analytics:Commission paid"), m.commissionAmount, {precision: 2, prefix: "¥"})}
              {this.metricCard(i18next.t("analytics:Withdrawals"), m.withdrawals)}
            </Row>
            <Row gutter={16}>
              {this.metricCard(i18next.t("analytics:Events today"), stats.totalToday, {span: 6})}
              {this.metricCard(i18next.t("analytics:Active users today"), stats.dauToday, {span: 6})}
              {this.metricCard(i18next.t("analytics:Total events"), stats.totalEvents, {span: 6})}
              {this.metricCard(i18next.t("analytics:Lookback days"), stats.days, {span: 6})}
            </Row>

            <Row gutter={16}>
              <Col span={14}>{this.renderTrend(stats.daily)}</Col>
              <Col span={10}>{this.renderFunnel(stats.funnel)}</Col>
            </Row>

            <Row gutter={16} style={{marginTop: "16px"}}>
              <Col span={8}>
                <Card size="small" title={i18next.t("analytics:Top events")}>
                  <Table rowKey="key" size="small" pagination={{pageSize: 8}} columns={breakdownCols("analytics:Event")} dataSource={stats.topEvents || []} loading={this.state.loading} />
                </Card>
              </Col>
              <Col span={8}>
                <Card size="small" title={i18next.t("analytics:By platform")}>
                  <Table rowKey="key" size="small" pagination={false} columns={breakdownCols("analytics:Platform")} dataSource={stats.byPlatform || []} loading={this.state.loading} />
                </Card>
              </Col>
              <Col span={8}>
                <Card size="small" title={i18next.t("analytics:By source")}>
                  <Table rowKey="key" size="small" pagination={false} columns={breakdownCols("analytics:Source")} dataSource={stats.bySource || []} loading={this.state.loading} />
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
