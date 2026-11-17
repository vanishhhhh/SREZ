-- CHECK expressions accept SQL UNKNOWN. Explicit NOT NULL predicates keep a
-- public list from ever having an absent capability digest/ciphertext.
ALTER TABLE wishlists ADD CONSTRAINT wishlist_public_link_complete CHECK (
    visibility <> 'public' OR (
        share_token_hash IS NOT NULL AND octet_length(share_token_hash) = 32
        AND share_token_sealed IS NOT NULL AND octet_length(share_token_sealed) >= 28
    )
);
