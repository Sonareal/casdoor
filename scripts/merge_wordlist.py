#!/usr/bin/env python3
"""把外部敏感词库合并进 conf/sensitive_words.txt。

手写词库永远补不全，正确的做法是从维护中的公开词库导入。但公开词库普遍偏
激进，直接 cat 进来会误杀正常昵称，所以这个脚本在写入前会：

  1. 归一化 + 去重（和 Go 侧 util.NormalizeForMatch 保持同样规则）
  2. 丢掉过短的词条（默认 <2 字），它们几乎必然误杀
  3. 拿一份"正常昵称语料"跑一遍，报告哪些新词会造成误杀
  4. 默认只报告不写入，确认后加 --apply 才落盘

用法：
    python3 scripts/merge_wordlist.py 外部词库.txt --section 涉政
    python3 scripts/merge_wordlist.py 外部词库.txt --section 涉政 --apply

外部词库格式：一行一词，# 开头为注释。
"""

import argparse
import re
import sys
import unicodedata
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
WORDLIST = REPO_ROOT / "conf" / "sensitive_words.txt"

# 与 util/sensitive.go 的 confusables 保持一致
CONFUSABLES = {"0": "o", "1": "i", "3": "e", "4": "a", "5": "s", "7": "t", "@": "a", "$": "s"}


def normalize(text: str) -> str:
    """必须与 Go 侧 util.NormalizeForMatch 行为一致。

    注意：这里没有实现繁->简折叠（Go 侧有）。对合并工具来说影响是它可能把
    繁体词当成新词而不是重复词，属于宁多勿少，可以接受。
    """
    text = unicodedata.normalize("NFKC", text).lower()
    out = []
    for ch in text:
        ch = CONFUSABLES.get(ch, ch)
        if ch.isalnum():
            out.append(ch)
    return "".join(out)


def parse_wordlist(path: Path):
    """返回 (词条 -> 所属分节, 白名单集合)。"""
    words, allow = {}, set()
    section, in_allow = "", False
    for line in path.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        if line.startswith("["):
            section = line.strip("[]").strip()
            in_allow = section.lower() in {"白名单", "例外", "allowlist", "allow list", "whitelist"}
            continue
        if in_allow:
            allow.add(line)
        else:
            words[line] = section
    return words, allow


def read_external(path: Path):
    out = []
    for line in path.read_text(encoding="utf-8", errors="replace").splitlines():
        line = line.strip()
        if line and not line.startswith("#"):
            # 常见公开词库用逗号或制表符分隔，一并拆开
            out.extend(p.strip() for p in re.split(r"[,\t，]", line) if p.strip())
    return out


# 正常昵称语料：新词若命中这里的任何一条，就是误杀。
# 这份语料要和 util/wordlist_test.go 的 allowed 列表保持同步扩充。
HAND_PICKED_NAMES = [
    "张小明", "李冰冰", "王大锤", "赵日天", "陈奕迅", "黄晓明", "黄文", "白娜",
    "刘德华", "周杰伦", "林志玲", "孙悟空", "李小龙", "小红帽", "灰姑娘",
    "小白兔", "海洋之心", "奥特曼超人", "爱学习的小明", "阳光少年", "快乐星球",
    "数学小天才", "英语小达人", "爱看书的猫", "彩虹独角兽", "天天向上",
    "处女座的我", "跳楼价清仓", "白粉笔", "大烟囱", "滑冰爱好者", "热血少年",
    "杀马特青年", "神仙水测评", "上门维修师傅", "冰糖葫芦", "黄山旅游",
    "大麻烦制造者", "中国好声音", "我爱北京", "人民教师", "解放军叔叔",
    "Alice Wang", "Bob Chen", "user2024", "player_01", "Skillful Coder",
    "Data Analysis", "First Class", "Peacock Blue", "Grape Juice", "Crisis Manager",
]


# 手挑的几十条不足以验证上千词的导入，所以再机器生成一批真实感的中文昵称：
# 常见姓 × 常见名用字，加上儿童向 App 里高频出现的昵称构词。命中其中任何一条
# 都算误杀 —— 宁可漏判，也不能让正常用户改不了昵称。
COMMON_SURNAMES = "王李张刘陈杨黄赵周吴徐孙马朱胡林郭何高罗郑梁谢宋唐许邓冯韩曹曾彭萧蔡潘田董袁于余叶蒋杜苏魏程吕丁沈任姚卢傅钟姜崔谭廖范汪陆金石戴贾韦夏邱方侯邹熊孟秦白江阎薛尹段雷黎史陶毛郝顾龚邵万钱严覃武戚莫孔向汤"
GIVEN_CHARS = "伟芳娜秀敏静丽强磊军洋勇艳杰娟涛明超秀霞平刚桂英华文瑞玉兰凤云建国志成新春晓东南西北天地日月星辰山川林森海洋波涛江河湖溪雨雪风霜梅竹菊松柏杨柳枫桐宁安康健乐欣悦怡然欢喜福禄寿禧鑫淼焱垚金木水火土仁义礼智信温良恭俭让"
NICKNAME_PARTS = [
    "小", "大", "老", "阿", "爱", "快乐", "阳光", "彩虹", "星星", "月亮", "太阳",
    "兔子", "猫咪", "小狗", "熊猫", "老虎", "狮子", "恐龙", "独角兽", "奥特曼",
    "学霸", "天才", "达人", "王者", "冠军", "队长", "博士", "老师", "同学",
    "读书", "写字", "画画", "唱歌", "跳舞", "游泳", "跑步", "篮球", "足球",
    "英语", "数学", "语文", "科学", "音乐", "美术", "编程", "机器人",
]


def build_corpus():
    corpus = list(HAND_PICKED_NAMES)
    for surname in COMMON_SURNAMES:
        for given in GIVEN_CHARS:
            corpus.append(surname + given)
            corpus.append(surname + given + given)
    for a in NICKNAME_PARTS:
        for b in NICKNAME_PARTS:
            if a != b:
                corpus.append(a + b)
        corpus.append(a)
    return corpus


ORDINARY_NAMES = build_corpus()


def build_substring_index(names, max_len=8):
    """子串 -> 一个示例昵称。

    逐条比对是 O(词数 × 语料条数)，导入几千词时太慢，所以预先把语料里所有
    长度 2..max_len 的子串摊平成索引，查一次是 O(1)。
    """
    index = {}
    for name in names:
        n = normalize(name)
        for start in range(len(n)):
            for end in range(start + 2, min(start + max_len, len(n)) + 1):
                index.setdefault(n[start:end], name)
    return index


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("source", type=Path, help="外部词库文件")
    ap.add_argument("--section", default="涉政", help="合并到哪个分节（默认 涉政）")
    ap.add_argument("--min-length", type=int, default=2, help="短于此长度的词条丢弃（默认 2）")
    ap.add_argument("--apply", action="store_true", help="真正写入；不加只做预演")
    args = ap.parse_args()

    if not args.source.exists():
        print(f"找不到词库文件: {args.source}", file=sys.stderr)
        return 1

    existing, allow = parse_wordlist(WORDLIST)
    existing_norm = {normalize(w) for w in existing}
    allow_norm = [normalize(a) for a in allow if normalize(a)]

    incoming = read_external(args.source)
    print(f"读入 {len(incoming)} 条，现有词库 {len(existing)} 条")
    substring_index = build_substring_index(ORDINARY_NAMES)
    print(f"误杀语料 {len(ORDINARY_NAMES)} 条昵称，展开成 {len(substring_index)} 个子串")

    fresh, too_short, duplicate, risky = [], [], [], []
    seen = set()
    for word in incoming:
        n = normalize(word)
        if not n:
            continue
        if len(n) < args.min_length:
            too_short.append(word)
            continue
        if n in existing_norm or n in seen:
            duplicate.append(word)
            continue
        seen.add(n)

        # 误杀检查：这个新词是否是某个正常昵称的子串
        example = substring_index.get(n)
        if example is not None:
            # 白名单可以救回来：命中片段完整落在某条白名单词组内部
            rescued = any(n in a and a in normalize(example) for a in allow_norm)
            if not rescued:
                risky.append((word, example))
                continue
        fresh.append(word)

    print(f"\n  可安全加入 : {len(fresh)}")
    print(f"  已存在跳过 : {len(duplicate)}")
    print(f"  过短丢弃   : {len(too_short)}" + (f"  例: {too_short[:8]}" if too_short else ""))
    print(f"  ⚠️ 会误杀   : {len(risky)}")
    for word, example in risky[:20]:
        print(f"      {word!r} 会误伤 {example!r}")
    if len(risky) > 20:
        print(f"      ...还有 {len(risky) - 20} 条")

    if risky:
        print("\n  误杀的词条不会被写入。若确认要收，请给它加 [白名单] 例外后重跑。")

    if not args.apply:
        print("\n预演模式，未写入。确认无误后加 --apply。")
        return 0

    if not fresh:
        print("\n没有可写入的新词条。")
        return 0

    text = WORDLIST.read_text(encoding="utf-8")
    header = f"[{args.section}]"
    block = "\n".join(fresh) + "\n"

    # 只认"独占一行"的分节头。用 text.index() 找会命中写在注释里的分节名
    # （例如 "见 [涉政] 一节的说明"），把词条插进注释块中间。
    match = re.search(rf"^{re.escape(header)}\s*$", text, re.MULTILINE)
    if match:
        # 跳过紧随其后的注释行，让新词插在注释之后
        idx = match.end() + 1
        while idx < len(text):
            line_end = text.find("\n", idx)
            if line_end == -1:
                break
            line = text[idx:line_end].strip()
            if line.startswith("#"):
                idx = line_end + 1
            else:
                break
        text = text[:idx] + block + text[idx:]
    else:
        text = text.rstrip("\n") + f"\n\n{header}\n" + block
    WORDLIST.write_text(text, encoding="utf-8")

    print(f"\n✅ 已写入 {len(fresh)} 条到 [{args.section}]")
    print("下一步：")
    print("  go test ./util/ -run ShippedWordList    # 误杀回归测试")
    print("  scp conf/sensitive_words.txt elejoai@192.168.2.12:~/casdoor/conf/")
    print("  （词库是热加载的，10 秒内生效，不需要重新部署）")
    return 0


if __name__ == "__main__":
    sys.exit(main())
