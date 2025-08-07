#!/usr/bin/env python3

import argparse
import re
from transformers import pipeline

CHUNK_SIZE = 1500
SUMMARY_MAX_LENGTH = 150
SUMMARY_MIN_LENGTH = 30
MIN_CHUNK_CHARS = 100

def split_text(text, max_length):
    return [text[i:i+max_length] for i in range(0, len(text), max_length)]

def clean_log_text(text):
    text = re.sub(r'\x1B(?:[@-Z\\-_]|\[[0-?]*[ -/]*[@-~])', '', text)  # remove ANSI
    text = re.sub(r'\s+', ' ', text)  # normalize whitespace
    return text.strip()

def main():
    parser = argparse.ArgumentParser(description="Summarize a CI log file.")
    parser.add_argument("logfile", type=str, help="Path to the log file to summarize")
    parser.add_argument("--model", type=str, default="sshleifer/distilbart-cnn-12-6", help="Summarization model to use")
    args = parser.parse_args()

    with open(args.logfile, "r", errors="ignore") as f:
        raw_text = f.read()

    cleaned = clean_log_text(raw_text)
    chunks = [c for c in split_text(cleaned, CHUNK_SIZE) if len(c.strip()) > MIN_CHUNK_CHARS]

    print(f"Summarizing {len(chunks)} chunks")
    summarizer = pipeline("summarization", model=args.model)

    all_summaries = []

    results = summarizer(
        chunks,
        max_length=SUMMARY_MAX_LENGTH,
        min_length=SUMMARY_MIN_LENGTH,
        do_sample=False
    )
    for i, result in enumerate(results):
        all_summaries.append(f"Chunk {i+1} summary: {result['summary_text']}")

    print("\n Final summary:")
    print("=" * 40)
    print("\n\n".join(all_summaries))

if __name__ == "__main__":
    main()
