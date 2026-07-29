import subprocess, tempfile, shutil, sys
from pathlib import Path
repo = Path("/Volumes/MyExternalDisk/Work/feiyu-admin/casdoor")

# 构造一个词库：分节名同时出现在注释里 —— 就是踩过的那个坑
fixture = """[白名单]
大麻烦

[A类]
# 说明里提到 [B类] 这个名字，不能被当成插入点
海洛因

[B类]
# 这是 B 类的注释行
已有词
"""
with tempfile.TemporaryDirectory() as d:
    d = Path(d)
    wl = repo / "conf" / "sensitive_words.txt"
    backup = wl.read_text(encoding="utf-8")
    try:
        wl.write_text(fixture, encoding="utf-8")
        (d / "new.txt").write_text("新词甲\n新词乙\n", encoding="utf-8")
        subprocess.run([sys.executable, str(repo/"scripts/merge_wordlist.py"),
                        str(d/"new.txt"), "--section", "B类", "--apply"],
                       capture_output=True, check=True)
        out = wl.read_text(encoding="utf-8")
    finally:
        wl.write_text(backup, encoding="utf-8")

lines = [l.rstrip() for l in out.splitlines()]
i_a = lines.index("[A类]"); i_b = lines.index("[B类]")
for w in ("新词甲", "新词乙"):
    assert w in lines, f"{w} 没写入"
    assert lines.index(w) > i_b, f"{w} 被插到 [B类] 之前了（注释匹配 bug 复发）"
assert lines.index("新词甲") > lines.index("# 这是 B 类的注释行"), "新词插在了注释之前"
assert i_a < i_b, "分节顺序被破坏"
assert "海洛因" in lines and lines.index("海洛因") < i_b, "[A类] 的词被移动了"
print("✅ merge_wordlist 插入位置回归测试通过")
