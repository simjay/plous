"""Custom Soft flow lettering with the smoother Level ends s.

The p comes directly from mark.svg. These paths supply the remaining letters.
"""

WIDTH, HEIGHT = 375, 124
ORIGIN = (-10, 23)
LETTERS = (
    ("l", 101, "M6 -7V46C6 60 12 68 27 68"),
    ("o", 148, "M6 65V40C6 25 17 17 32 17C48 17 59 27 59 43C59 59 48 68 32 68H25"),
    ("u", 227, "M6 17V43C6 60 15 68 31 68C47 68 57 59 57 43V17"),
    ("s", 304,
     "M52 17H25C13.5 17 7 22 7 29.75C7 37.5 13.5 42.5 25 42.5H38"
     "C49.5 42.5 56 47.5 56 55.25C56 63 49.5 68 38 68H8"),
)
