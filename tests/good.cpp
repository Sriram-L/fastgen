#include <stdio.h>

// Correct solution: sum the n numbers.
int main() {
    long long n = 0;
    if (scanf("%lld", &n) != 1) {
        return 0;
    }

    long long sum = 0;
    for (long long i = 0; i < n; i++) {
        long long x = 0;
        scanf("%lld", &x);
        sum += x;
    }

    printf("%lld\n", sum);
    return 0;
}
