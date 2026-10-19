#include "kuznechik.h"

std::vector<short> StringToVec(const std::string& val) {
    static std::string symbols = "0123456789abcdef";
    std::vector<short> result;
    result.reserve(val.size() / 2);
    for (int i = 0; i < val.size(); i += 2) {
        auto pos_1 = symbols.find(val[i]);
        auto pos_2 = symbols.find(val[i + 1]);
        result.push_back(pos_1 * 16 + pos_2);
    }
    return result;}

std::string DataToString(const short* values) {
    static std::string symbols = "0123456789abcdef";
    std::string result;
    for (int i = 0; i < 16; ++i) {
        int val_1 = values[i] / 16;
        int val_2 = values[i] % 16;
        result += symbols[val_1];
        result += symbols[val_2];
    }
    return result;}

Kuznyechik::Kuznyechik(const short* master_key) {
    powers[0] = 1;
    inv_powers[1] = 0;
    for (int i = 1; i < (1 << 8); ++i) {
        powers[i] = powers[i - 1] << 1;
        if (powers[i] >= (1 << 8)) {
            powers[i] ^= kGaluaPoly;
        }
        inv_powers[powers[i]] = i;
    }
    short base_matrix[16][16];
    short temp_base_matrix[16][16];
    for (int i = 0; i < 16; ++i) {
        base_matrix[0][i] = linear_coefs[i];
    }
    for (int i = 1; i < 16; ++i) {
        for (int j = 0; j < 16; ++j) {
            base_matrix[i][j] = (i == j + 1);
        }
    }
    short power_matrix[16][16];
    for (int i = 0; i < 16; ++i) {
        for (int j = 0; j < 16; ++j) {
            power_matrix[i][j] = (i == j);
        }
    }
    for (int pw = 1; pw <= 16; ++pw) {
        for (int i = 0; i < 16; ++i) {
            for (int j = 0; j < 16; ++j) {
                temp_base_matrix[i][j] = 0;
                for (int k = 0; k < 16; ++k) {
                    temp_base_matrix[i][j] ^= GaluaMult(base_matrix[i][k], power_matrix[k][j]);
                }
            }
        }
        for (int i = 0; i < 16; ++i) {
            for (int j = 0; j < 16; ++j) {
                power_matrix[i][j] = temp_base_matrix[i][j];
            }
        }
    }

    for (short i = 0; i < 256; ++i) {
        inv_permutation[permutation[i]] = i;
    }

    for (short start_byte = 0; start_byte < 256; ++start_byte) {
        for (int j = 0; j < 16; ++j) {
            for (int i = 0; i < 2; ++i) {
                precalc[start_byte][j][i] = 0;
            }
            for (int i = 0; i < 16; ++i) {
                precalc[start_byte][j][i >> 3] <<= 8;
                precalc[start_byte][j][i >> 3] += static_cast<uint64_t>(GaluaMult(power_matrix[i][j], start_byte));
            }
        }
    }

    for (short i = 0; i < 32; ++i) {
        for (short j = 0; j < 15; ++j) {
            c_coefs[i][j] = 0;
        }
        c_coefs[i][15] = i + 1;
        L(c_coefs[i]);
    }
    for (int i = 0; i < 16; ++i) {
        k_coefs[0][i] = master_key[i];
        k_coefs[1][i] = master_key[i + 16];
    }
    for (int i = 1; i <= 4; ++i) {
        for (int j = 0; j < 16; ++j) {
            k_coefs[i * 2][j] = k_coefs[i * 2 - 2][j];
            k_coefs[i * 2 + 1][j] = k_coefs[i * 2 - 1][j];
        }
        for (int j = 0; j < 8; ++j) {
            F(c_coefs[(i - 1) * 8 + j], k_coefs[i * 2], k_coefs[i * 2 + 1]);
        }
    }
    for (int i = 0; i < 10; ++i) {
        big_k_coefs[i][0] = big_k_coefs[i][1] = 0;
        for (int j = 0; j < 16; ++j) {
            big_k_coefs[i][j >> 3] <<= 8;
            big_k_coefs[i][j >> 3] += static_cast<uint64_t>(k_coefs[i][j]);
        }
    }
}

void Kuznyechik::Encrypt(short* a) {
    for (int i = 0; i < 16; ++i) {
        temp_a[i] = a[i] ^ k_coefs[0][i];
    }
    for (short i = 1; i < 10; ++i) {
        LSX(big_k_coefs[i][0], big_k_coefs[i][1]);
    }
    for (int i = 0; i < 16; ++i) {
        a[i] = temp_a[i];
    }
}

void Kuznyechik::Decrypt(short* a) {
    for (int i = 9; i > 0; --i) {
        X(a, k_coefs[i]);
        inv_L(a);
        inv_S(a);
    }
    X(a, k_coefs[0]);
}

void Kuznyechik::inv_S(short* a) {
    for (int i = 0; i < 16; ++i) {
        a[i] = inv_permutation[a[i]];
    }
}

void Kuznyechik::inv_L(short* a) {
    for (int i = 0; i < 16; ++i) {
        l_temp_a[i] = a[i];
    }
    for (int i = 0; i < 16; ++i) {
        inv_R(l_temp_a + i);
    }
    for (int i = 0; i < 16; ++i) {
        a[i] = l_temp_a[16 + i];
    }
}

void Kuznyechik::inv_R(short* a) {
    a[16] = a[0];
    a[16] = Linear(a + 1);
}

short Kuznyechik::Linear(short* a) {
    short result = 0;
    for (int i = 0; i < 16; ++i) {
        result ^= GaluaMult(linear_coefs[i], a[i]);
    }
    return result;
}

void Kuznyechik::LSX(uint64_t k1, uint64_t k2) {
    static uint64_t result[2];
    result[0] = k1;
    result[1] = k2;
    for (short i = 0; i < 16; ++i) {
        l_temp_a[i] = permutation[temp_a[i]];
    }
    for (short j = 0; j < 16; ++j) {
        for (short i = 0; i < 2; ++i) {
            result[i] ^= precalc[l_temp_a[j]][j][i];
        }
    }
    for (short i = 15; i >= 0; --i) {
        temp_a[i] = result[i >> 3] & 255;
        result[i >> 3] >>= 8;
    }
}

void Kuznyechik::F(const short* k, short* a_1, short* a_0) {
    for (short i = 0; i < 16; ++i) {
        temp_a[i] = a_1[i];
    }
    SmallLSX(temp_a, k);
    X(temp_a, a_0);
    for (int i = 0; i < 16; ++i) {
        a_0[i] = a_1[i];
        a_1[i] = temp_a[i];
    }
}

void Kuznyechik::X(short* a, const short* k) {
    for (short i = 0; i < 16; ++i) {
        a[i] ^= k[i];
    }
}

void Kuznyechik::L(short* a) {
    uint64_t result[2];
    result[0] = result[1] = 0;
    for (int i = 0; i < 2; ++i) {
        for (int j = 0; j < 16; ++j) {
            result[i] ^= precalc[a[j]][j][i];
        }
    }
    for (int i = 15; i >= 0; --i) {
        a[i] = result[i >> 3] & 255;
        result[i >> 3] >>= 8;
    }
}

void Kuznyechik::SmallLSX(short* a, const short* k) {
    X(a, k);
    LS(a);
}

void Kuznyechik::LS(short* a) {
    static uint64_t result[2];
    result[0] = result[1] = 0;
    for (short i = 0; i < 16; ++i) {
        l_temp_a[i] = permutation[a[i]];
    }
    for (short j = 0; j < 16; ++j) {
        for (short i = 0; i < 2; ++i) {
            result[i] ^= precalc[l_temp_a[j]][j][i];
        }
    }
    for (short i = 15; i >= 0; --i) {
        a[i] = result[i >> 3] & 255;
        result[i >> 3] >>= 8;
    }
}

short Kuznyechik::GaluaMult(short x, short y) {
    if (x == 0 || y == 0) return 0;
    int inv_pos_x = inv_powers[x];
    int inv_pos_y = inv_powers[y];
    int pos = inv_pos_x + inv_pos_y;
    if (pos >= 255) {
        pos -= 255;
    }
    return powers[pos];
}
