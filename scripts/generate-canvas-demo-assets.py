#!/usr/bin/env python3
"""Generate the small PDF/audio/video fixtures used by the Canvas demo project."""

from __future__ import annotations

import argparse
import subprocess
from pathlib import Path

from reportlab.lib.colors import HexColor
from reportlab.lib.pagesizes import A4
from reportlab.lib.styles import ParagraphStyle, getSampleStyleSheet
from reportlab.lib.units import mm
from reportlab.pdfbase import pdfmetrics
from reportlab.pdfbase.ttfonts import TTFont
from reportlab.platypus import Paragraph, SimpleDocTemplate, Spacer, Table, TableStyle


FONT_REGULAR = Path("/System/Library/Fonts/Supplemental/Arial.ttf")
FONT_BOLD = Path("/System/Library/Fonts/Supplemental/Arial Bold.ttf")


def generate_pdf(output: Path) -> None:
    pdfmetrics.registerFont(TTFont("DemoArial", FONT_REGULAR))
    pdfmetrics.registerFont(TTFont("DemoArialBold", FONT_BOLD))
    styles = getSampleStyleSheet()
    title = ParagraphStyle(
        "DemoTitle",
        parent=styles["Title"],
        fontName="DemoArialBold",
        fontSize=24,
        leading=29,
        textColor=HexColor("#17324D"),
        spaceAfter=8 * mm,
    )
    heading = ParagraphStyle(
        "DemoHeading",
        parent=styles["Heading2"],
        fontName="DemoArialBold",
        fontSize=15,
        leading=19,
        textColor=HexColor("#C26B3A"),
        spaceBefore=4 * mm,
        spaceAfter=2 * mm,
    )
    body = ParagraphStyle(
        "DemoBody",
        parent=styles["BodyText"],
        fontName="DemoArial",
        fontSize=10.5,
        leading=15,
        textColor=HexColor("#263746"),
    )
    doc = SimpleDocTemplate(
        str(output),
        pagesize=A4,
        leftMargin=20 * mm,
        rightMargin=20 * mm,
        topMargin=18 * mm,
        bottomMargin=18 * mm,
        title="Эхо Предела — протокол полевого теста",
        author="Writers Studio demo",
    )
    story = [
        Paragraph("Эхо Предела", title),
        Paragraph("Протокол полевого теста: звуковая навигация", heading),
        Paragraph(
            "Этот короткий документ служит PDF-референсом на демонстрационной Canvas-доске. "
            "Он описывает одну проверяемую гипотезу vertical slice и переносится внутри файла .wstudio.",
            body,
        ),
        Spacer(1, 5 * mm),
        Table(
            [
                [
                    Paragraph("Гипотеза", ParagraphStyle("TableHead", parent=body, fontName="DemoArialBold", textColor=HexColor("#FFFFFF"))),
                    Paragraph("Наблюдаемый сигнал", ParagraphStyle("TableHead2", parent=body, fontName="DemoArialBold", textColor=HexColor("#FFFFFF"))),
                ],
                [
                    Paragraph("Игрок понимает цену громкого эха", body),
                    Paragraph("Перед первым хищником меняет режим импульса", body),
                ],
                [
                    Paragraph("Карта остаётся полезной после сдвига острова", body),
                    Paragraph("Сравнивает новый контур со старым следом", body),
                ],
                [
                    Paragraph("Возвращение ощущается осмысленным", body),
                    Paragraph("Называет ресурс, который хочет сохранить", body),
                ],
            ],
            colWidths=[62 * mm, 98 * mm],
            style=TableStyle(
                [
                    ("FONTNAME", (0, 0), (-1, 0), "DemoArialBold"),
                    ("FONTNAME", (0, 1), (-1, -1), "DemoArial"),
                    ("FONTSIZE", (0, 0), (-1, -1), 9),
                    ("LEADING", (0, 0), (-1, -1), 12),
                    ("BACKGROUND", (0, 0), (-1, 0), HexColor("#17324D")),
                    ("TEXTCOLOR", (0, 0), (-1, 0), HexColor("#FFFFFF")),
                    ("BACKGROUND", (0, 1), (-1, -1), HexColor("#F2EEE8")),
                    ("TEXTCOLOR", (0, 1), (-1, -1), HexColor("#263746")),
                    ("GRID", (0, 0), (-1, -1), 0.5, HexColor("#9A8778")),
                    ("VALIGN", (0, 0), (-1, -1), "TOP"),
                    ("LEFTPADDING", (0, 0), (-1, -1), 7),
                    ("RIGHTPADDING", (0, 0), (-1, -1), 7),
                    ("TOPPADDING", (0, 0), (-1, -1), 7),
                    ("BOTTOMPADDING", (0, 0), (-1, -1), 7),
                ]
            ),
        ),
        Paragraph("Как использовать на доске", heading),
        Paragraph(
            "Откройте PDF-карточку, сравните её с вопросами плейтеста, затем переключитесь на "
            "Dependency graph или запустите диагностику. Файл должен пережить экспорт, импорт и повторный экспорт без потерь.",
            body,
        ),
    ]
    doc.build(story)


def run_ffmpeg(args: list[str]) -> None:
    subprocess.run(["ffmpeg", "-hide_banner", "-loglevel", "error", "-y", *args], check=True)


def generate_audio(output: Path) -> None:
    run_ffmpeg(
        [
            "-f",
            "lavfi",
            "-i",
            "sine=frequency=174:duration=4:sample_rate=44100",
            "-af",
            "afade=t=in:st=0:d=0.5,afade=t=out:st=3.2:d=0.8,volume=0.18",
            "-codec:a",
            "libmp3lame",
            "-b:a",
            "96k",
            str(output),
        ]
    )


def generate_video(output: Path) -> None:
    signal = (
        "drawbox=x=70:y=70:w=500:h=220:color=#17324d@0.85:t=fill,"
        "drawbox=x=80+85*t:y=250:w=110:h=4:color=#67e8f9@0.9:t=fill,"
        "drawbox=x=115:y=110:w=410:h=4:color=#c26b3a@0.75:t=fill,"
        "drawbox=x=115:y=155:w=310:h=3:color=#f2eee8@0.55:t=fill,"
        "drawbox=x=115:y=190:w=360:h=3:color=#f2eee8@0.35:t=fill"
    )
    run_ffmpeg(
        [
            "-f",
            "lavfi",
            "-i",
            "color=c=#101820:s=640x360:d=4:r=24",
            "-f",
            "lavfi",
            "-i",
            "sine=frequency=87:duration=4:sample_rate=44100",
            "-vf",
            signal,
            "-af",
            "volume=0.08,afade=t=out:st=3.3:d=0.7",
            "-c:v",
            "libx264",
            "-pix_fmt",
            "yuv420p",
            "-c:a",
            "aac",
            "-shortest",
            "-movflags",
            "+faststart",
            str(output),
        ]
    )


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=Path, default=Path("tmp/canvas-demo-assets"))
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)
    generate_pdf(args.output / "echo-frontier-playtest.pdf")
    generate_audio(args.output / "echo-frontier-signal.mp3")
    generate_video(args.output / "echo-frontier-field-log.mp4")
    for item in sorted(args.output.iterdir()):
        print(f"{item}\t{item.stat().st_size} bytes")


if __name__ == "__main__":
    main()
