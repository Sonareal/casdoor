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

package util

import (
	"testing"
)

// These tests run against the real shipped word list rather than a fixture, so
// that editing conf/sensitive_words.txt cannot silently break coverage or start
// rejecting ordinary names.

const wordListPath = "../conf/sensitive_words.txt"

func loadShippedWordList(t *testing.T) {
	t.Helper()

	if err := InitSensitiveFilter(wordListPath); err != nil {
		t.Fatal(err)
	}
	if SensitiveWordCount() == 0 {
		t.Fatalf("the shipped word list at %s loaded zero words", wordListPath)
	}
	t.Cleanup(func() { _ = InitSensitiveFilter("") })
}

func TestShippedWordListBlocks(t *testing.T) {
	loadShippedWordList(t)

	// One or more representative entries per category, written the way an
	// abuser would actually type them into a nickname field.
	blocked := map[string][]string{
		"毒品":   {"售卖海洛因", "出冰毒", "代购大麻", "嗑药少年", "笑气批发", "求购芬太尼", "her0in"},
		"色情":   {"看黄片", "约炮达人", "招嫖联系", "恋童癖", "裸聊室", "porn king", "N S F W"},
		"暴力":   {"我要杀人", "灭门惨案", "校园霸凌王", "教你自杀", "恐怖分子", "分尸现场", "suicide squad"},
		"违禁品":  {"出售枪支", "办证刻章", "银行卡四件套"},
		"赌博":   {"澳门赌场荷官", "六合彩内部"},
		"诈骗":   {"官方客服", "唯一客服小李", "系统管理员"},
		"广告":   {"加微信abc", "刷单返利"},
		"辱骂":   {"傻逼一个", "fuck you"},
		"繁体变体": {"販毒集團", "強姦犯", "自殺", "綁架"},
	}

	for category, samples := range blocked {
		for _, sample := range samples {
			t.Run(category+"/"+sample, func(t *testing.T) {
				if word, hit := ContainsSensitiveWord(sample); !hit {
					t.Errorf("%q should be blocked but passed", sample)
				} else if word == "" {
					t.Errorf("%q was blocked without reporting which word matched", sample)
				}
			})
		}
	}
}

// The expensive failure mode is not a missed word, it is blocking ordinary
// users. Every name here is one a real person could plausibly pick, and several
// deliberately contain a substring of a banned word.
func TestShippedWordListDoesNotBlockOrdinaryNames(t *testing.T) {
	loadShippedWordList(t)

	allowed := []string{
		// ordinary Chinese names
		"张小明", "李冰冰", "王大锤", "赵日天", "陈奕迅", "黄晓明", "黄文", "白娜",
		// nicknames a child might pick
		"小白兔", "海洋之心", "奥特曼超人", "爱学习的小明", "阳光少年", "快乐星球",
		"数学小天才", "英语小达人", "爱看书的猫", "彩虹独角兽",
		// contain a substring of a banned word but are innocent
		"处女座的我",  // 处女 is inside 处女座
		"跳楼价清仓",  // 跳楼 is inside 跳楼价
		"白粉笔",    // 白粉
		"大烟囱",    // 大烟
		"滑冰爱好者",  // 溜冰 / 冰
		"热血少年",   // 血
		"杀马特青年",  // 杀
		"神仙水测评",  // 神仙水
		"上门维修师傅", // 上门服务
		"冰糖葫芦",   // 冰
		"黄山旅游",   // 黄
		"大麻烦制造者", // 大麻 —— 靠 [白名单] 里的 "大麻烦" 救回来
		// English handles with banned substrings
		"Skillful Coder", // kill inside skill
		"Data Analysis",  // anal inside analysis
		"First Class",    // ass inside class
		"Peacock Blue",   // cock inside peacock
		"Grape Juice",    // rape inside grape
		"Crisis Manager", // isis inside crisis
		"Alice Wang", "Bob Chen", "user2024", "player_01",
	}

	for _, name := range allowed {
		t.Run(name, func(t *testing.T) {
			if word, hit := ContainsSensitiveWord(name); hit {
				t.Errorf("ordinary name %q was blocked by the word %q — false positives are worse than misses here", name, word)
			}
		})
	}
}
