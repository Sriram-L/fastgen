#include <stdio.h>

// Deliberately buggy solution: adds one for every testcase that contains the
// sentinel value 1000. The generator only emits it for seeds above 500, so
// the first 500 testcases pass and the animation keeps running for a while.
int main() {
    long long n = 0;
    if (scanf("%lld", &n) != 1) {
        return 0;
    }

    long long sum = 0;
    int buggy_case = 0;
    for (long long i = 0; i < n; i++) {
        long long x = 0;
        scanf("%lld", &x);
        if (x == 1000) {
            buggy_case = 1;
        }
        sum += x;
    }

    if (buggy_case) {
        sum += 1;
    }

    printf("%lld\n", sum);
    return 0;
}
