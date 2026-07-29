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
ORDINARY_NAMES = [
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

        # 误杀检查：这个新词是否会命中正常昵称，且没有被白名单救回
        hits = [name for name in ORDINARY_NAMES
                if n in normalize(name)
                and not any(n in a and a in normalize(name) for a in allow_norm)]
        if hits:
            risky.append((word, hits))
        else:
            fresh.append(word)

    print(f"\n  可安全加入 : {len(fresh)}")
    print(f"  已存在跳过 : {len(duplicate)}")
    print(f"  过短丢弃   : {len(too_short)}" + (f"  例: {too_short[:8]}" if too_short else ""))
    print(f"  ⚠️ 会误杀   : {len(risky)}")
    for word, hits in risky[:20]:
        print(f"      {word!r} 会误伤 {hits[:3]}")
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
    if header in text:
        idx = text.index(header) + len(header) + 1
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
