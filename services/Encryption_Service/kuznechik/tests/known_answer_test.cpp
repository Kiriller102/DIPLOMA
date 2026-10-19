#include <iostream>
#include <stdexcept>
#include "../src/kuznechik/kuznechik.h"

// RFC 7801, sections 5.5 and 5.6.
int main() {
    auto key = StringToVec("8899aabbccddeeff0011223344556677fedcba98765432100123456789abcdef");
    auto block = StringToVec("1122334455667700ffeeddccbbaa9988");
    Kuznyechik cipher(key.data());
    cipher.Encrypt(block.data());
    if (DataToString(block.data()) != "7f679d90bebc24305a468d42b9d4edcd") {
        std::cerr << "RFC encryption vector mismatch\n"; return 1;
    }
    cipher.Decrypt(block.data());
    if (DataToString(block.data()) != "1122334455667700ffeeddccbbaa9988") {
        std::cerr << "RFC decryption vector mismatch\n"; return 1;
    }
    for (const auto& bad : {"0", "zz"}) {
        try { StringToVec(bad); return 1; } catch (const std::invalid_argument&) { }
    }
    for (int i = 0; i < 256; ++i) {
        std::vector<short> sample(16, i);
        cipher.Encrypt(sample.data()); cipher.Decrypt(sample.data());
        if (sample != std::vector<short>(16, i)) return 1;
    }
    std::cout << "RFC 7801 vectors, round trips and invalid hex: OK\n";
}
