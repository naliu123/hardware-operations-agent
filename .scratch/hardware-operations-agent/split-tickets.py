"""Create missing local tickets from the approved implementation plan."""

from pathlib import Path
import re

root = Path(__file__).resolve().parents[2]
source = root / "docs/design/hardware-operations-agent-implementation-plan.md"
text = source.read_text()
dependencies = {
    task: re.findall(r"[A-Z]{2}-\d{2}", raw)
    for task, raw in re.findall(
        r"^\| ([A-Z]{2}-\d{2}) \| ([^|]+) \| [^|]+ \|$", text, re.M
    )
}
sections = list(re.finditer(r"^### ([A-Z]{2}-\d{2})：(.*)$", text, re.M))
numbers = {match.group(1): index for index, match in enumerate(sections, 1)}
destination = Path(__file__).parent / "issues"
destination.mkdir(exist_ok=True)
for index, match in enumerate(sections, 1):
    task, title = match.groups()
    end = sections[index].start() if index < len(sections) else len(text)
    body = text[match.end():end].split("\n## ", 1)[0].strip()
    target = destination / f"{index:02d}-{task.lower()}.md"
    if target.exists():
        continue
    blocked = ", ".join(
        f"{numbers[dep]:02d} ({dep})" for dep in dependencies[task]
    ) or "None"
    header = (
        f"# {index:02d}: {task} {title}\n\n"
        f"**Task:** {task}\n\n"
        f"**Blocked by:** {blocked}\n\n"
        f"**Status:** {'in-progress' if task == 'QA-01' else 'ready-for-agent'}\n\n"
        "**Spec:** [总体设计](../../../docs/design/"
        "hardware-operations-agent-overall-design.md) · "
        "[Go + Eino 技术设计](../../../docs/design/"
        "hardware-operations-agent-technical-design.md)\n\n"
    )
    # Tickets sit three directories beneath the workspace root.
    target.write_text(header + body + "\n")
    print(target.relative_to(root))
