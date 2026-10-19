#include <iostream>

#include "../src/kuznechik/kuznechik.h"

const int kIterations = 625000;

int main() {
    std::string ans = "74ca32d959885c410983ad8c8427c9fa";

    auto master_key = StringToVec("8899aabbccddeeff0011223344556677fedcba98765432100123456789abcdef");
    Kuznyechik kuznyechik(master_key.data());

    // auto open_text = StringToVec("1122334455667700ffeeddccbbaa9988");
    auto open_text = StringToVec("48656c6c6f20576f726c6421");
    std::cout << "Open text: " << DataToString(open_text.data()) << '\n';

    auto start = std::chrono::steady_clock::now();
    for (int i = 0; i < kIterations; ++i) {
        kuznyechik.Encrypt(open_text.data());
    }
    auto stop = std::chrono::steady_clock::now();
    auto duration = std::chrono::duration_cast<std::chrono::milliseconds>(stop - start);
    std::cout << "Encrypt time: " << duration.count() << " ms\n";

    std::cout << "Encrypted text: " << DataToString(open_text.data()) << '\n';

    start = std::chrono::steady_clock::now();
    for (int i = 0; i < kIterations; ++i) {
        kuznyechik.Decrypt(open_text.data());
    }
    stop = std::chrono::steady_clock::now();
    duration = std::chrono::duration_cast<std::chrono::milliseconds>(stop - start);
    std::cout << "Decrypt time: " << duration.count() << " ms\n";

    std::cout << "Decrypted text: " << DataToString(open_text.data()) << '\n';

    return 0;
}
