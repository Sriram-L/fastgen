#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>

// After this seed the generator adds a sentinel value that only the buggy
// solution mishandles. With the default --seed 1 this means the first 500
// testcases pass and testcase #500 (seed 501) fails.
#define FAIL_AFTER_SEED 500ULL

static uint64_t next_random(uint64_t& state) {
    state += 0x9E3779B97F4A7C15ULL;
    uint64_t z = state;
    z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9ULL;
    z = (z ^ (z >> 27)) * 0x94D049BB133111EBULL;
    return z ^ (z >> 31);
}

int main(int argc, char** argv) {
    uint64_t seed = argc > 1 ? strtoull(argv[1], NULL, 10) : 0;
    uint64_t state = seed;

    int n = 5 + static_cast<int>(next_random(state) % 6);
    printf("%d\n", n);
    for (int i = 0; i < n; i++) {
        long long value = static_cast<long long>(next_random(state) % 100);
        if (seed > FAIL_AFTER_SEED && i == 0) {
            value = 1000;
        }
        printf("%lld%c", value, i + 1 == n ? '\n' : ' ');
    }
    return 0;
}
