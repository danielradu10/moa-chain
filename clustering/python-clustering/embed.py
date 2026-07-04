#!/usr/bin/env python3

from sentence_transformers import SentenceTransformer


MODEL_NAME = "sentence-transformers/all-MiniLM-L6-v2"
MODEL_REVISION = "1110a243fdf4706b3f48f1d95db1a4f5529b4d41"
MODEL_DEVICE = "cpu"


def embed_answers(answers: list[str]) -> list[list[float]]:
    for index, answer in enumerate(answers):
        if not isinstance(answer, str) or not answer:
            raise ValueError(f"answer at index {index} must be a non-empty string")

    if not answers:
        return []

    model = SentenceTransformer(
        MODEL_NAME,
        revision=MODEL_REVISION,
        device=MODEL_DEVICE,
    )

    embeddings = model.encode(answers, normalize_embeddings=True)
    return embeddings.tolist()
