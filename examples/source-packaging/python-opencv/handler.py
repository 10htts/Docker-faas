import sys

import cv2
import numpy as np


def main():
    _ = sys.stdin.read()
    image = np.zeros((120, 160, 3), dtype=np.uint8)
    cv2.rectangle(image, (10, 10), (150, 110), (255, 255, 255), 2)
    edges = cv2.Canny(image, 80, 160)
    count = int(np.sum(edges > 0))
    print(f"opencv edges: pixels={count}, shape={edges.shape}")


if __name__ == "__main__":
    main()
