#!/usr/bin/env python3
"""
Генерация docs/KNOWLEDGE_BASE.md из FAQ_ITEMS без обращения к внешним API Notion.
"""

import os
import sys

# Добавляем каталог со скриптом в пути поиска
script_dir = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, script_dir)

# Обеспечиваем фиктивный токен для оффлайн-импорта модулей FAQ
os.environ.setdefault("NOTION_TOKEN", "offline_dummy_token")

import sync_to_notion as s

HUB_ID = "4012e6735a41815fb1cbd4f95937849b"
DB_ID = "4012e6735a41819f989bf707734342e9"
VERSION = "v1.14.2"

kb_path = os.path.join(script_dir, "..", "..", "docs", "KNOWLEDGE_BASE.md")

with open(kb_path, "w", encoding="utf-8") as f:
    f.write("# База знаний и FAQ: Деперсонализация ПД (152-ФЗ)\n\n")
    f.write(f"**Версия продукта:** `{VERSION}`  \n")
    f.write(f"**Notion Hub:** `https://app.notion.com/{HUB_ID}`  \n")
    f.write(f"**Notion FAQ Database:** `https://app.notion.com/{DB_ID}`  \n\n")
    f.write("---\n\n")
    f.write("## Оглавление базы знаний\n\n")
    for idx, item in enumerate(s.FAQ_ITEMS, 1):
        heading_title = f"{idx}. {item['question']}"
        anchor = s.slugify(heading_title)
        f.write(f"{idx}. [{item['question']}](#{anchor})\n")
    f.write("\n---\n\n")
    for idx, item in enumerate(s.FAQ_ITEMS, 1):
        f.write(f"### {idx}. {item['question']}\n\n")
        f.write(f"- **Категория:** `{item['category']}`\n")
        f.write(f"- **Аудитория:** `{item['audience']}`\n")
        tags_str = ", ".join(f"`{t}`" for t in item["tags"])
        f.write(f"- **Теги:** {tags_str}\n")
        f.write(f"- **Актуальная версия:** `{VERSION}`\n\n")
        f.write(f"{item['body']}\n\n---\n\n")

print(f"-> Успешно сгенерирован {kb_path} (версия {VERSION})")
