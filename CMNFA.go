/*
  Copyright (C) THL A29 Limited, a Tencent company. All rights reserved.

  SPDX-License-Identifier: Apache-2.0
*/

/*
参照CMNFA合约标准实现：
https://git.chainmaker.org.cn/contracts/standard/-/blob/master/draft/CM-CS-221221-NFA.md
*/

package main

import (
	"encoding/json"
  	"encoding/hex"
	"errors"
	"fmt"
	"strings"
  
  	"github.com/btcsuite/btcutil/base58"
	"chainmaker.org/chainmaker/contract-utils/standard"

	"chainmaker.org/chainmaker/common/v2/crypto"
	"chainmaker.org/chainmaker/contract-sdk-go/v2/pb/protogo"
	"chainmaker.org/chainmaker/contract-sdk-go/v2/sandbox"
	"chainmaker.org/chainmaker/contract-sdk-go/v2/sdk"
	"chainmaker.org/chainmaker/contract-utils/address"
	"chainmaker.org/chainmaker/contract-utils/safemath"
)

const (
	paramAdmin        = "admin"
	paramCategoryName = "categoryName"
	paramCategoryURI  = "categoryURI"
	paramCategory     = "category"
	paramFrom         = "from"
	paramTo           = "to"
	paramTokenId      = "tokenId"
	paramTokenIds     = "tokenIds"
	paramMetadata     = "metadata"
	paramTokens       = "tokens"
	paramIsApproval   = "isApproval"
	paramStandardName = "standardName"

	categoryMapName                = "categoryMap"
	categoryTotalSupplyMapName     = "categoryTotalSupplyMap"
	balanceInfoMapName             = "balanceInfoMap"
	accountMapName                 = "accountInfoMap"
	tokenOwnerMapName              = "tokenOwnerMap"
	tokenInfoMapName               = "tokenInfoMap"
	tokenApprovalMapName           = "tokenApprovalMap"
	tokenApprovalForAllMapName     = "tokenApprovalForAllMap"
	tokenApprovalByCategoryMapName = "tokenApprovalByCategoryMap"

	adminStoreKey        = "admin"
	metadataStoreKey     = "metadata"
	categoryNameStoreKey = "categoryName"
	totalSupplyStoreKey  = "TotalSupply"

	nfaStandardName = standard.ContractStandardNameCMNFA
)

var _ standard.CMNFA = (*CMNFAContract)(nil)
var _ standard.CMBC = (*CMNFAContract)(nil)
var _ standard.CMNFAOption = (*CMNFAContract)(nil)

// CMNFAContract contract for NFA
type CMNFAContract struct {
}

// InitContract install contract func
func (c *CMNFAContract) InitContract() protogo.Response {
	err := c.updateNFAInfo(true)
	if err != nil {
		sdk.Instance.Errorf("Init contract err: %s", err)
		return sdk.Error(err.Error())
	}
	return sdk.Success([]byte("Init contract success"))
}

// UpgradeContract upgrade contract func
func (c *CMNFAContract) UpgradeContract() protogo.Response {
	err := c.updateNFAInfo(false)
	if err != nil {
		sdk.Instance.Errorf("Upgrade contract err: %s", err)
		return sdk.Error(err.Error())
	}
	return sdk.Success([]byte("Upgrade contract success"))
}

// UpgradeContract upgrade contract func
func (c *CMNFAContract) updateNFAInfo(isInitContract bool) error {
	args := sdk.Instance.GetArgs()
	categoryName := args[paramCategoryName]
	categoryURI := string(args[paramCategoryURI])
	admin := string(args[paramAdmin])

	if len(categoryName) > 0 {
		categoryMap, err := c.getCategoryStoreMap()
		if err != nil {
			return fmt.Errorf("new storeMap of category failed, err:%s", err)
		}
		err = categoryMap.Set([]string{string(categoryName)}, []byte(categoryURI))
		if err != nil {
			return fmt.Errorf("categoryMap set default category failed, err:%s", err)
		}
	}

	if len(admin) > 0 {
		err := sdk.Instance.PutStateFromKey(adminStoreKey, admin)
		if err != nil {
			return fmt.Errorf("save admin failed, err:%s", err)
		}
	} else if isInitContract {
		origin, err := sdk.Instance.Origin()
		if err != nil {
			return err
		}
		err = sdk.Instance.PutStateFromKey(adminStoreKey, origin)
		if err != nil {
			return fmt.Errorf("save admin failed, err:%s", err)
		}
	}
	return nil
}

// InvokeContract the entry func of invoke contract func
// nolint: gocyclo
func (c *CMNFAContract) InvokeContract(method string) protogo.Response {
	if len(method) == 0 {
		return sdk.Error("method of param should not be empty")
	}
	switch method {
	case "Standards":
		return c.standardsCore()
	case "SupportStandard":
		return c.supportStandardCore()
	case "Mint":
		return c.mintCore()
	case "MintBatch":
		return c.mintBatchCore()
	case "SetApproval":
		return c.setApprovalCore()
	case "SetApprovalForAll":
		return c.setApprovalForAllCore()
	case "TransferFrom":
		return c.transferFromCore()
	case "TransferFromBatch":
		return c.transferFromBatchCore()
	case "OwnerOf":
		return c.ownerOfCore()
	case "TokenURI":
		return c.tokenURICore()
	// below methods are optional
    case "GetAdmin":
      	return c.GetAdmin()
	case "SetApprovalByCategory":
		return c.setApprovalByCategoryCore()
	case "CreateOrSetCategory":
		return c.createOrSetCategoryCore()
	case "Burn":
		return c.burnCore()
	case "GetCategoryByName":
		return c.getCategoryByNameCore()
	case "GetCategoryByTokenId":
		return c.getCategoryByTokenIdCore()
	case "TotalSupply":
		return c.totalSupplyCore()
	case "TotalSupplyOfCategory":
		return c.totalSupplyOfCategoryCore()
	case "BalanceOf":
		return c.balanceOfCore()
	case "AccountTokens":
		return c.accountTokensCore()
	case "TokenMetadata":
		return c.tokenMetadataCore()
	default:
		return sdk.Error("Invalid method")
	}
}

func (c *CMNFAContract) standardsCore() protogo.Response {
	standards := c.Standards()
	standardsBytes, err := json.Marshal(standards)
	if err != nil {
		return sdk.Error(err.Error())
	}
	return sdk.Success(standardsBytes)
}

func (c *CMNFAContract) supportStandardCore() protogo.Response {
	args := sdk.Instance.GetArgs()
	standardName := string(args[paramStandardName])
	isSupport := c.SupportStandard(standardName)
	if isSupport {
		return sdk.Success([]byte(standard.TrueString))
	}
	return sdk.Success([]byte(standard.FalseString))
}

// Standards returns standard strings which contain "CMNFA"
func (c *CMNFAContract) Standards() (standards []string) {
	return []string{nfaStandardName}
}

// SupportStandard returns true if standardName equals "CMNFA"
func (c *CMNFAContract) SupportStandard(standardName string) bool {
	return standardName == nfaStandardName
}

func (c *CMNFAContract) mintCore() protogo.Response {
	args := sdk.Instance.GetArgs()
	to := string(args[paramTo])
	tokenId := string(args[paramTokenId])
	categoryName := string(args[paramCategoryName])
	metadata := args[paramMetadata]
	err := c.verifyToken(to, tokenId, categoryName)
	if err != nil {
		return sdk.Error(err.Error())
	}
	sender, err := sdk.Instance.Sender()
	if err != nil {
		return sdk.Error(fmt.Sprintf("get sender failed, err:%s", err))
	}
	admin, _, err := sdk.Instance.GetStateFromKeyWithExists(adminStoreKey)
	if err != nil {
		return sdk.Error(fmt.Sprintf("get admin failed, err:%s", err))
	}
	if sender != admin {
		return sdk.Error("only admin can mint tokens")
	}
	err = c.Mint(to, tokenId, categoryName, metadata)
	if err != nil {
		return sdk.Error(err.Error())
	}
	//time.Sleep(time.Second * 2)
	return sdk.Success([]byte("Mint success"))
}

// Mint a token, Obligatory.
// @param to, the owner address of the token. Obligatory.
// @param tokenId, the id of the token. Obligatory.
// @param categoryName, the name of the category. If categoryName is empty, the token's category
// will be the default category. Optional.
// @param metadata, the metadata of the token. Optional.
// @return error, the error msg if some error occur.
// @event, topic: 'mint'; data: to, tokenId, categoryName, metadata
func (c *CMNFAContract) Mint(to, tokenId, categoryName string, metadata []byte) error {
	err := c.increaseBalanceByOne(to)
	if err != nil {
		return err
	}
	err = c.increaseTotalSupplyByOne()
	if err != nil {
		return err
	}
	err = c.increaseTotalSupplyOfCategoryByOne(categoryName)
	if err != nil {
		return err
	}
	err = c.setTokenOwner(to, tokenId)
	if err != nil {
		return err
	}

	err = c.setAccountToken(address.ZeroAddr, to, tokenId)
	if err != nil {
		return err
	}

	err = c.setTokenInfo(tokenId, categoryName, metadata)
	if err != nil {
		return err
	}

	c.EmitMintEvent(to, tokenId, categoryName, string(metadata))
	return nil
}

func (c *CMNFAContract) mintBatchCore() protogo.Response {
	args := sdk.Instance.GetArgs()
	tokens := args[paramTokens]
	if len(tokens) == 0 {
		return sdk.Error("invalid tokens parameter")
	}
	nfas := make([]standard.NFA, 0)
	err := json.Unmarshal(tokens, &nfas)
	if err != nil {
		sdk.Instance.Errorf("unmarshal tokens failed, content:%s, err:%s", tokens, err)
		return sdk.Error(fmt.Sprintf("unmarshal tokens failed, err:%s", err))
	}
	for _, nfa := range nfas {
		err = c.verifyToken(nfa.To, nfa.TokenId, nfa.CategoryName)
		if err != nil {
			return sdk.Error(err.Error())
		}
	}

	sender, err := sdk.Instance.Sender()
	if err != nil {
		return sdk.Error(fmt.Sprintf("get sender failed, err:%s", err))
	}
	admin, _, err := sdk.Instance.GetStateFromKeyWithExists(adminStoreKey)
	if err != nil {
		return sdk.Error(fmt.Sprintf("get admin failed, err:%s", err))
	}
	if sender != admin {
		return sdk.Error("only admin can mint tokens")
	}
	err = c.MintBatch(nfas)
	if err != nil {
		return sdk.Error(err.Error())
	}
	return sdk.Success([]byte("MintBatch success"))
}

// MintBatch mint nfa tokens batch. Obligatory.
// @param tokens, the tokens to mint. Obligatory.
// @return error, the error msg if some error occur.
// @event, topic: 'mintBatch'; data: tokens
func (c *CMNFAContract) MintBatch(tokens []standard.NFA) error {
	for _, token := range tokens {
		err := c.Mint(token.To, token.TokenId, token.CategoryName, token.Metadata)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *CMNFAContract) verifyToken(to, tokenId, categoryName string) error {
	if !address.IsValidAddress(to) || address.IsZeroAddress(to) {
		return fmt.Errorf("mint to invalid address")
	}
	if len(tokenId) == 0 {
		return fmt.Errorf("invalid tokenId")
	}
	minted, err := c.minted(tokenId)
	if err != nil {
		return err
	}
	if minted {
		return fmt.Errorf("duplicated token")
	}
	if len(categoryName) == 0 {
		return fmt.Errorf("invalid category name")
	}

	categoryMap, err := c.getCategoryStoreMap()
	if err != nil {
		return fmt.Errorf("new storeMap of category failed, err:%s", err)
	}
	exist, err := categoryMap.Exist([]string{categoryName})
	if err != nil {
		return fmt.Errorf("get category failed, err:%s", err)
	}
	if !exist {
		return fmt.Errorf("category not exist")
	}
	return nil
}

func (c *CMNFAContract) setApprovalCore() protogo.Response {
	args := sdk.Instance.GetArgs()
	tokenId := string(args[paramTokenId])
	to := string(args[paramTo])
	isApproval := string(args[paramIsApproval])

	if len(tokenId) == 0 {
		return sdk.Error("invalid tokenId")
	}
	if address.IsZeroAddress(to) || !address.IsValidAddress(to) {
		return sdk.Error("invalid to address")
	}
	if isApproval != standard.TrueString && isApproval != standard.FalseString {
		return sdk.Error("isApproval only can be true or false")
	}
	// check owner
	owner, err := c.OwnerOf(tokenId)
	if err != nil {
		return sdk.Error(err.Error())
	}

	// check approve info
	sender, err := sdk.Instance.Sender()
	if err != nil {
		return sdk.Error(fmt.Sprintf("get sender failed, err:%s", err))
	}
	if owner != sender {
		return sdk.Error("only owner can set approve")
	}
	err = c.SetApproval(owner, to, tokenId, isApproval == standard.TrueString)
	if err != nil {
		return sdk.Error(err.Error())
	}

	return sdk.Success([]byte("SetApproval success"))
}

// SetApproval approve or cancel approve token to 'to' account. Obligatory.
// @param to, destination approve to. Obligatory.
// @param tokenId, the token id. Obligatory.
// @param isApproval, to approve or to cancel approve
// @return error, the error msg if some error occur.
// @event, topic: 'SetApproval'; data: to, tokenId, isApproval
func (c *CMNFAContract) SetApproval(owner, to, tokenId string, isApproval bool) error {
	tokenApproveInfo, err := c.getTokenApprovalStoreMap()
	if err != nil {
		return fmt.Errorf("new storeMap of tokenApproveMap failed, err:%s", err)
	}
	if isApproval {
		err = tokenApproveInfo.Set([]string{tokenId}, []byte(to))
	} else {
		err = tokenApproveInfo.Del([]string{tokenId})
	}
	if err != nil {
		return fmt.Errorf("set approve failed, err:%s", err)
	}

	c.EmitSetApprovalEvent(owner, to, tokenId, isApproval)

	return nil
}

func (c *CMNFAContract) setApprovalForAllCore() protogo.Response {
	args := sdk.Instance.GetArgs()
	to := string(args[paramTo])
	isApproval := string(args[paramIsApproval])
	if address.IsZeroAddress(to) || !address.IsValidAddress(to) {
		return sdk.Error("invalid to address")
	}
	if isApproval != standard.TrueString && isApproval != standard.FalseString {
		return sdk.Error("isApprove only can be true or false")
	}

	sender, err := sdk.Instance.Sender()
	if err != nil {
		return sdk.Error(fmt.Sprintf("get sender failed, err:%s", err))
	}
	err = c.SetApprovalForAll(sender, to, isApproval == standard.TrueString)
	if err != nil {
		return sdk.Error(fmt.Sprintf("set approve for all failed, err:%s", err))
	}
	return sdk.Success([]byte("SetApprovalForAll success"))
}

// SetApprovalForAll approve or cancel approve all token to 'to' account. Obligatory.
// @param to, destination address approve to. Obligatory.
// @isApprove, true means approve and false means cancel approve. Obligatory.
// @return error, the error msg if some error occur.
// @event, topic: 'SetApprovalForAll'; data: to, isApproval
func (c *CMNFAContract) SetApprovalForAll(owner, to string, isApproval bool) error {
	operatorApprovalInfo, err := c.getTokenApprovalForAllStoreMap()
	if err != nil {
		return fmt.Errorf("new storemap of operatorApprove failed, err:%s", err)
	}
	if isApproval {
		err = operatorApprovalInfo.Set([]string{owner, to}, []byte(standard.TrueString))
	} else {
		err = operatorApprovalInfo.Del([]string{owner, to})
	}
	if err != nil {
		return fmt.Errorf("set operator approve failed, err:%s", err)
	}

	c.EmitSetApprovalForAllEvent(owner, to, isApproval)

	return nil
}

func (c *CMNFAContract) GetAdmin() protogo.Response {
    admin, _, err := sdk.Instance.GetStateFromKeyWithExists(adminStoreKey)
    if err != nil {
        return sdk.Error("get admin failed")
    }
    return sdk.Success([]byte(admin))
}

func (c *CMNFAContract) getApproved(tokenId string) (bool, error) {
	tokenApproveInfo, err := c.getTokenApprovalStoreMap()
	if err != nil {
		return false, err
	}
	approveTo, err := tokenApproveInfo.Get([]string{tokenId})
	if err != nil {
		return false, err
	}

	return !address.IsZeroAddress(string(approveTo)) && address.IsValidAddress(string(approveTo)), nil
}

func (c *CMNFAContract) isApprovedForAll(owner, sender string) (bool, error) {
	tokenApprovalForAllMap, err := c.getTokenApprovalForAllStoreMap()
	if err != nil {
		return false, err
	}
	val, err := tokenApprovalForAllMap.Get([]string{owner, sender})
	if err != nil {
		return false, fmt.Errorf("get approved val from approve info failed, err:%s", err)
	}
	return string(val) == standard.TrueString, nil
}

func (c *CMNFAContract) isApprovedByCategory(owner, sender, categoryName string) (bool, error) {
	tokenApprovalForAllMap, err := c.getTokenApprovalByCategoryStoreMap()
	if err != nil {
		return false, err
	}
	val, err := tokenApprovalForAllMap.Get([]string{owner, sender, categoryName})
	if err != nil {
		return false, fmt.Errorf("get approved val from approve info failed, err:%s", err)
	}
	return string(val) == standard.TrueString, nil
}

func (c *CMNFAContract) transferFromCore() protogo.Response {
	args := sdk.Instance.GetArgs()
	from := string(args[paramFrom])
	to := string(args[paramTo])
	tokenId := string(args[paramTokenId])
	if address.IsZeroAddress(from) || !address.IsValidAddress(from) {
		return sdk.Error("invalid from address")
	}
	if address.IsZeroAddress(to) || !address.IsValidAddress(to) {
		return sdk.Error("invalid to address")
	}
	if len(tokenId) == 0 {
		return sdk.Error("invalid tokenId")
	}
	sender, err := sdk.Instance.Sender()
	if err != nil {
		return sdk.Error(fmt.Sprintf("get sender failed, err:%s", err))
	}
	isApprovedOrOwner, err := c.isApprovedOrOwner(sender, tokenId)
	if err != nil {
		return sdk.Error(fmt.Sprintf("check isApprovedOrOwner failed, err:%s", err))
	}
	if !isApprovedOrOwner {
		return sdk.Error("sender is not token owner or approved")
	}
	err = c.TransferFrom(from, to, tokenId)
	if err != nil {
		return sdk.Error(err.Error())
	}
	return sdk.Success([]byte("TransferFrom success"))
}

// TransferFrom transfer single token after approve. Obligatory.
// @param from, owner account of token. Obligatory.
// @param to, destination account transferred to. Obligatory.
// @param tokenId, the token being transferred. Obligatory.
// @return error, the error msg if some error occur.
// @event, topic: 'TransferFrom'; data: from, to, tokenId
func (c *CMNFAContract) TransferFrom(from, to, tokenId string) error {
	// delete token approve
	tokenApproveInfo, err := c.getTokenApprovalStoreMap()
	if err != nil {
		return fmt.Errorf("new storeMap of token approve failed, err:%s", err)
	}
	err = tokenApproveInfo.Del([]string{tokenId})
	if err != nil {
		return fmt.Errorf("delete token approve failed, err:%s", err)
	}

	// update "from" balance count
	err = c.decreaseBalanceByOne(from)
	if err != nil {
		return err
	}

	// update "to" balance count
	err = c.increaseBalanceByOne(to)
	if err != nil {
		return err
	}

	// update token owner
	err = c.setTokenOwner(to, tokenId)
	if err != nil {
		return err
	}

	err = c.setAccountToken(from, to, tokenId)
	if err != nil {
		return err
	}

	c.EmitTransferFromEvent(from, to, tokenId)

	return nil
}

func (c *CMNFAContract) transferFromBatchCore() protogo.Response {
	args := sdk.Instance.GetArgs()
	from := string(args[paramFrom])
	to := string(args[paramTo])
	tokenIdsBytes := args[paramTokenIds]
	if address.IsZeroAddress(from) || !address.IsValidAddress(from) {
		return sdk.Error("invalid from address")
	}
	if address.IsZeroAddress(to) || !address.IsValidAddress(to) {
		return sdk.Error("invalid to address")
	}
	if len(tokenIdsBytes) == 0 {
		return sdk.Error("invalid tokenIds")
	}
	tokenIds := make([]string, 0)
	err := json.Unmarshal(tokenIdsBytes, &tokenIds)
	if err != nil {
		return sdk.Error("invalid tokenIds")
	}
	sender, err := sdk.Instance.Sender()
	if err != nil {
		return sdk.Error(fmt.Sprintf("get sender failed, err:%s", err))
	}
	if len(tokenIds) == 0 {
		return sdk.Error("invalid tokenIds")
	}
	for _, tokenId := range tokenIds {
		if len(tokenId) == 0 {
			return sdk.Error("invalid tokenId")
		}
		var isApprovedOrOwner bool
		isApprovedOrOwner, err = c.isApprovedOrOwner(sender, tokenId)
		if err != nil {
			return sdk.Error(err.Error())
		}
		if !isApprovedOrOwner {
			return sdk.Error("only owner or approved account can transfer token")
		}
	}
	err = c.TransferFromBatch(from, to, tokenIds)
	if err != nil {
		return sdk.Error(err.Error())
	}
	return sdk.Success([]byte("TransferFrom success"))

}

// TransferFromBatch transfer tokens after approve. Obligatory.
// @param from, owner account of token. Obligatory.
// @param to, destination account transferred to. Obligatory.
// @param tokenIds, the tokens being transferred. Obligatory.
// @return error, the error msg if some error occur.
// @event, topic: 'TransferFromBatch'; data: from, to, tokenIds
func (c *CMNFAContract) TransferFromBatch(from, to string, tokenIds []string) error {
	for _, tokenId := range tokenIds {
		err := c.TransferFrom(from, to, tokenId)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *CMNFAContract) ownerOfCore() protogo.Response {
	args := sdk.Instance.GetArgs()
	if len(args) == 0 {
		return sdk.Error("param tokenId is needed")
	}
	tokenId := string(args["tokenId"])
	if len(tokenId) == 0 {
		return sdk.Error("invalid tokenId")
	}
	owner, err := c.OwnerOf(tokenId)
	if err != nil {
		return sdk.Error(err.Error())
	}
	return sdk.Success([]byte(owner))
}

// OwnerOf get the owner of a token. Obligatory.
// @param tokenId, the token which will be queried. Obligatory.
// @return account, the token's account.
// @return err, the error msg if some error occur.
func (c *CMNFAContract) OwnerOf(tokenId string) (account string, err error) {
	tokenOwnerMap, err := c.getTokenOwnerStoreMap()
	if err != nil {
		return "", fmt.Errorf("New storeMap of tokenOwnerMap failed, err:%s", err)
	}

	owner, err := tokenOwnerMap.Get([]string{tokenId})
	if err != nil {
		return "", err
	}

	return string(owner), nil
}

func (c *CMNFAContract) tokenURICore() protogo.Response {
	args := sdk.Instance.GetArgs()
	if len(args) == 0 {
		return sdk.Error("param tokenId is needed")
	}
	tokenId := string(args[paramTokenId])
	if len(tokenId) == 0 {
		return sdk.Error("invalid tokenId")
	}
	uri, err := c.TokenURI(tokenId)
	if err != nil {
		return sdk.Error(err.Error())
	}

	return sdk.Success([]byte(uri))
}

// TokenURI get the URI of the token. a token's uri consists of CategoryURI and tokenId. Obligatory.
// @param tokenId, tokenId be queried. Obligatory.
// @return uri, the uri of the token.
// @return err, the error msg if some error occur.
func (c *CMNFAContract) TokenURI(tokenId string) (uri string, err error) {
	category, err := c.GetCategoryByTokenId(tokenId)
	if err != nil {
		return "", err
	}
	if category == nil {
		return "", errors.New("category not found")
	}
	//todo:format
	//http://chainmaker.org.cn/token/%s.json
	//http://chainmaker.org.cn/token/1000.json
	//http://www.chainmaker.org, 1000
	//http://www.chainmaker.org/1000
	//return fmt.Sprintf(category.CategoryURI, tokenId), nil
	return category.CategoryURI + "/" + tokenId, nil
}

// EmitMintEvent emits Mint event
func (c *CMNFAContract) EmitMintEvent(to, tokenId, categoryName, metadata string) {
	sdk.Instance.EmitEvent("Mint", []string{address.ZeroAddr, to, tokenId, categoryName, metadata})
}

// EmitSetApprovalEvent emits SetApproval event
func (c *CMNFAContract) EmitSetApprovalEvent(owner, to, tokenId string, isApproval bool) {
	if isApproval {
		sdk.Instance.EmitEvent("SetApproval", []string{owner, to, tokenId, standard.TrueString})
	} else {
		sdk.Instance.EmitEvent("SetApproval", []string{owner, to, tokenId, standard.FalseString})
	}
}

// EmitSetApprovalForAllEvent emits SetApprovalForAll event
func (c *CMNFAContract) EmitSetApprovalForAllEvent(owner, to string, isApproval bool) {
	if isApproval {
		sdk.Instance.EmitEvent("SetApprovalForAll", []string{owner, to, standard.TrueString})
	} else {
		sdk.Instance.EmitEvent("SetApprovalForAll", []string{owner, to, standard.FalseString})
	}
}

// EmitTransferFromEvent emits TransferFrom event
func (c *CMNFAContract) EmitTransferFromEvent(from, to, tokenId string) {
	sdk.Instance.EmitEvent("TransferFrom", []string{from, to, tokenId})
}

func (c *CMNFAContract) setApprovalByCategoryCore() protogo.Response {
	args := sdk.Instance.GetArgs()
	if len(args) == 0 {
		return sdk.Error("param to and categoryName are needed")
	}
	to := string(args[paramTo])
	categoryName := string(args[paramCategoryName])
	isApproval := string(args[paramIsApproval])
	if !address.IsValidAddress(to) || address.IsZeroAddress(to) {
		return sdk.Error("invalid to address")
	}
	if isApproval != standard.TrueString && isApproval != standard.FalseString {
		return sdk.Error("isApprove only can be true or false")
	}
	category, err := c.GetCategoryByName(categoryName)
	if err != nil || category == nil {
		return sdk.Error("invalid categoryName")
	}
	sender, err := sdk.Instance.Sender()
	if err != nil {
		return sdk.Error(err.Error())
	}
	err = c.SetApprovalByCategory(sender, to, categoryName, isApproval == standard.TrueString)
	if err != nil {
		return sdk.Error(err.Error())
	}
	return sdk.Success([]byte("setApprovalByCategoryCore success"))
}

// SetApprovalByCategory approve or cancel approve tokens of category to 'to' account. Optional.
// @param to, destination address approve to. Obligatory.
// @categoryName, the category of tokens. Obligatory.
// @isApproval, to approve or to cancel approve. Obligatory.
// @return error, the error msg if some error occur.
// @event, topic: 'SetApprovalByCategory'; data: to, categoryName, isApproval
func (c *CMNFAContract) SetApprovalByCategory(owner, to, categoryName string, isApproval bool) error {
	tokenApprovalByCategoryMap, err := c.getTokenApprovalByCategoryStoreMap()
	if err != nil {
		return err
	}
	if isApproval {
		err = tokenApprovalByCategoryMap.Set([]string{owner, to, categoryName}, []byte(standard.TrueString))
	} else {
		err = tokenApprovalByCategoryMap.Del([]string{owner, to, categoryName})
	}

	if err != nil {
		return err
	}

	c.EmitSetApprovalByCategoryEvent(owner, to, categoryName, isApproval)

	return nil
}

func (c *CMNFAContract) createOrSetCategoryCore() protogo.Response {
	args := sdk.Instance.GetArgs()
	if len(args) == 0 {
		return sdk.Error("param category are needed")
	}
	categoryBytes := args[paramCategory]
	if len(categoryBytes) == 0 {
		return sdk.Error("invalid category")
	}
	category := &standard.Category{}
	err := json.Unmarshal(categoryBytes, category)
	if err != nil {
		return sdk.Error(err.Error())
	}
	err = c.CreateOrSetCategory(category)
	if err != nil {
		return sdk.Error(err.Error())
	}
	return sdk.Success([]byte("CreateOrSetCategory success"))
}

// CreateOrSetCategory create ore reset a category. Optional.
// @param categoryName, the category name. Obligatory.
// @param categoryURI, the category uri. Obligatory.
// @return error, the error msg if some error occur.
// @event, topic: 'CreateOrSetCategory'; data: category
func (c *CMNFAContract) CreateOrSetCategory(category *standard.Category) error {
	cm, err := c.getCategoryStoreMap()
	if err != nil {
		return err
	}

	c.EmitCreateOrSetCategoryEvent(category.CategoryName, category.CategoryURI)

	return cm.Set([]string{category.CategoryName}, []byte(category.CategoryURI))
}

func (c *CMNFAContract) burnCore() protogo.Response {
	args := sdk.Instance.GetArgs()
	if len(args) == 0 {
		return sdk.Error("param tokenId is needed")
	}
	tokenId := string(args[paramTokenId])
	if len(tokenId) == 0 {
		return sdk.Error("invalid tokenId")
	}
	sender, err := sdk.Instance.Sender()
	if err != nil {
		return sdk.Error(fmt.Sprintf("get sender failed, err:%s", err))
	}
	approved, err := c.isApprovedOrOwner(sender, tokenId)
	if err != nil {
		return sdk.Error(fmt.Sprintf("check approved failed, err:%s", err))
	}
	if !approved {
		return sdk.Error("only owner or approved user can Burn the token")
	}
	err = c.Burn(tokenId)
	if err != nil {
		return sdk.Error(err.Error())
	}
	return sdk.Success([]byte("Burn success"))
}

// Burn burn a token
// @param tokenId
// @event, topic: 'Burn'; data: tokenId
func (c *CMNFAContract) Burn(tokenId string) error {
	// delete token approve
	tokenApproveInfo, err := c.getTokenApprovalStoreMap()
	if err != nil {
		return fmt.Errorf("new storeMap of token approve failed, err:%s", err)
	}
	err = tokenApproveInfo.Del([]string{tokenId})
	if err != nil {
		return fmt.Errorf("delete token approve failed, err:%s", err)
	}

	// update "owner" balance count
	owner, err := c.OwnerOf(tokenId)
	if err != nil {
		return err
	}
	err = c.decreaseBalanceByOne(owner)
	if err != nil {
		return err
	}

	// update token owner
	err = c.setTokenOwner(address.ZeroAddr, tokenId)
	if err != nil {
		return err
	}

	err = c.setAccountToken(owner, address.ZeroAddr, tokenId)
	if err != nil {
		return err
	}

	c.EmitBurnEvent(tokenId)

	return nil
}

func (c *CMNFAContract) getCategoryByNameCore() protogo.Response {
	args := sdk.Instance.GetArgs()
	if len(args) == 0 {
		return sdk.Error("param to and categoryName are needed")
	}
	categoryName := string(args[paramCategoryName])
	if len(categoryName) == 0 {
		return sdk.Error("invalid categoryName")
	}
	category, err := c.GetCategoryByName(categoryName)
	if err != nil {
		return sdk.Error(err.Error())
	}
	if category == nil {
		return sdk.Error("category not exist")
	}
	categoryBytes, err := json.Marshal(category)
	if err != nil {
		return sdk.Error(err.Error())
	}
	return sdk.Success(categoryBytes)
}

// GetCategoryByName get specific category by name. Optional.
// @param categoryName, the name of the category. Obligatory.
// @return category, the category returned.
// @return err, the error msg if some error occur.
func (c *CMNFAContract) GetCategoryByName(categoryName string) (*standard.Category, error) {
	categoryMap, err := c.getCategoryStoreMap()
	if err != nil {
		return nil, err
	}
	uri, err := categoryMap.Get([]string{categoryName})
	if err != nil {
		return nil, err
	}
	if uri == nil {
		return nil, nil
	}
	return &standard.Category{
		CategoryName: categoryName,
		CategoryURI:  string(uri),
	}, nil
}

func (c *CMNFAContract) getCategoryByTokenIdCore() protogo.Response {
	args := sdk.Instance.GetArgs()
	if len(args) == 0 {
		return sdk.Error("param tokenId needed")
	}
	tokenId := string(args[paramTokenId])
	if len(tokenId) == 0 {
		return sdk.Error("invalid categoryName")
	}
	category, err := c.GetCategoryByTokenId(tokenId)
	if err != nil {
		return sdk.Error(err.Error())
	}

	categoryBytes, err := json.Marshal(category)
	if err != nil {
		return sdk.Error(err.Error())
	}
	return sdk.Success(categoryBytes)
}

// GetCategoryByTokenId get a specific category by tokenId. Optional.
// @param tokenId, the names of category to be queried. Obligatory.
// @return category, the result queried.
// @return err, the error msg if some error occur.
func (c *CMNFAContract) GetCategoryByTokenId(tokenId string) (category *standard.Category, err error) {
	categoryName, err := c.getCategoryNameByTokenId(tokenId)
	if err != nil {
		return nil, err
	}
	category, err = c.GetCategoryByName(categoryName)
	if err != nil || category == nil {
		return nil, err
	}
	return category, nil
}

func (c *CMNFAContract) getCategoryNameByTokenId(tokenId string) (string, error) {
	tokenInfoMap, err := c.getTokenInfoStoreMap()
	if err != nil {
		return "", err
	}
	categoryName, err := tokenInfoMap.Get([]string{tokenId, categoryNameStoreKey})
	if err != nil {
		return "", err
	}
	return string(categoryName), nil
}

func (c *CMNFAContract) totalSupplyCore() protogo.Response {
	totalSupply, err := c.TotalSupply()
	if err != nil {
		return sdk.Error(err.Error())
	}
	return sdk.Success([]byte(totalSupply.ToString()))
}

// TotalSupply get total token supply of this contract. Obligatory.
// @return TotalSupply, the total token supply value returned.
// @return err, the error msg if some error occur.
func (c *CMNFAContract) TotalSupply() (*safemath.SafeUint256, error) {
	totalSupplyStr, _, err := sdk.Instance.GetStateFromKeyWithExists(totalSupplyStoreKey)
	if err != nil {
		return nil, err
	}
	totalSupply, ok := safemath.ParseSafeUint256(totalSupplyStr)
	if !ok {
		return nil, fmt.Errorf("the total supply in store is invalid")
	}
	return totalSupply, nil
}

func (c *CMNFAContract) totalSupplyOfCategoryCore() protogo.Response {
	args := sdk.Instance.GetArgs()
	if len(args) == 0 {
		return sdk.Error("param categoryName is needed")
	}
	categoryName := string(args[paramCategoryName])
	if len(categoryName) == 0 {
		return sdk.Error("invalid categoryName")
	}
	totalSupply, err := c.TotalSupplyOfCategory(categoryName)
	if err != nil {
		return sdk.Error(err.Error())
	}
	return sdk.Success([]byte(totalSupply.ToString()))
}

// TotalSupplyOfCategory get total token supply of the category. Obligatory.
// @param category, the category of tokens. Obligatory.
// @return TotalSupply, the total token supply value returned.
// @return err, the error msg if some error occur.
func (c *CMNFAContract) TotalSupplyOfCategory(categoryName string) (*safemath.SafeUint256, error) {
	categoryTotalSupplyMap, err := c.getCategoryTotalSupplyStoreMap()
	if err != nil {
		return nil, err
	}
	totalSupplyOfCategoryStr, err := categoryTotalSupplyMap.Get([]string{categoryName})
	if err != nil {
		return nil, err
	}
	totalSupplyOfCategory, ok := safemath.ParseSafeUint256(string(totalSupplyOfCategoryStr))
	if !ok {
		return nil, fmt.Errorf("invalid TotalSupply of category in store")
	}
	return totalSupplyOfCategory, nil
}

// BalanceOf return token count of the account
func (c *CMNFAContract) balanceOfCore() protogo.Response {
	args := sdk.Instance.GetArgs()
	if len(args) == 0 {
		return sdk.Error("param account is needed")
	}
	account := string(args["account"])
	if len(account) == 0 {
		return sdk.Error("Param account should not be empty")
	}
	if !address.IsValidAddress(account) {
		return sdk.Error("invalid account address")
	}
	if address.IsZeroAddress(account) {
		return sdk.Error("address zero is not a valid owner")
	}
	balance, err := c.BalanceOf(account)
	if err != nil {
		return sdk.Error(err.Error())
	}

	return sdk.Success([]byte(balance.ToString()))

}

// BalanceOf get total token number of the account. Optional
// @param account, the account which will be queried. Obligatory.
// @return balance, the token number of the account.
// @return err, the error msg if some error occur.
func (c *CMNFAContract) BalanceOf(account string) (*safemath.SafeUint256, error) {
	balanceInfo, err := c.getBalanceInfoStoreMap()
	if err != nil {
		return nil, fmt.Errorf("New storeMap of balanceInfo failed, err:%s", err)
	}

	balanceCount, err := c.getBalance(balanceInfo, account)
	if err != nil {
		return nil, fmt.Errorf("Get balance failed, err:%s", err)
	}
	return balanceCount, nil
}

func (c *CMNFAContract) accountTokensCore() protogo.Response {
	args := sdk.Instance.GetArgs()
	if len(args) == 0 {
		return sdk.Error("param account is needed")
	}
	account := string(args["account"])
	if !address.IsValidAddress(account) || address.IsZeroAddress(account) {
		return sdk.Error("invalid account")
	}
	tokens, err := c.AccountTokens(account)
	if err != nil {
		return sdk.Error(err.Error())
	}
	ats := &standard.AccountTokens{
		Account: account,
		Tokens:  tokens,
	}
	var atsBytes []byte
	atsBytes, err = json.Marshal(ats)
	if err != nil {
		return sdk.Error(err.Error())
	}
	return sdk.Success(atsBytes)
}

// AccountTokens get the token list of the account. Optional
// @param account, the account which will be queried. Obligatory.
// @return tokenId, the list of tokenId.
// @return err, the error msg if some error occur.
func (c *CMNFAContract) AccountTokens(account string) ([]string, error) {
	am, err := c.getAccountStoreMap()
	if err != nil {
		return nil, fmt.Errorf("new store map of token info failed, err:%s", err)
	}
	rs, err := am.NewStoreMapIteratorPrefixWithKey([]string{account})
	if err != nil {
		return nil, fmt.Errorf("new store map iterator of project info failed, err: %s", err)
	}

	tokens := make([]string, 0)
	for {
		if !rs.HasNext() {
			break
		}
		var item string
		item, _, _, err = rs.Next()
		if err != nil {
			return nil, fmt.Errorf("iterator next failed, err: %s", err)
		}
		itemId := strings.TrimPrefix(strings.TrimPrefix(item, accountMapName), account)
		if len(itemId) == 0 {
			return nil, fmt.Errorf("invalid itemId")
		}
		tokens = append(tokens, itemId)
	}
	return tokens, nil
}

func (c *CMNFAContract) tokenMetadataCore() protogo.Response {
	args := sdk.Instance.GetArgs()
	if len(args) == 0 {
		return sdk.Error("tokenId is needed")
	}
	tokenId := string(args["tokenId"])
	if len(tokenId) == 0 {
		return sdk.Error("invalid tokenId")
	}
	metadata, err := c.TokenMetadata(tokenId)
	if err != nil {
		return sdk.Error(err.Error())
	}
	return sdk.Success(metadata)
}

// TokenMetadata get the metadata of a token. Optional.
// @param tokenId, tokenId which will be queried.
// @return metadata, the metadata of the token.
// @return err, the error msg if some error occur.
func (c *CMNFAContract) TokenMetadata(tokenId string) ([]byte, error) {
	ti, err := c.getTokenInfoStoreMap()
	if err != nil {
		return nil, fmt.Errorf("new store map of token info failed, err:%s", err)
	}

	metadata, err := ti.Get([]string{tokenId, metadataStoreKey})
	if err != nil {
		return nil, fmt.Errorf("set metadata of erc721Info failed, err:%s", err)
	}

	sdk.Instance.Debugf("TokenMetadata is %s", string(metadata))

	return metadata, nil
}

// EmitBurnEvent emits Burn event
func (c *CMNFAContract) EmitBurnEvent(tokenId string) {
	sdk.Instance.EmitEvent("Burn", []string{tokenId})
}

// EmitSetApprovalByCategoryEvent emits SetApprovalByCategory event
func (c *CMNFAContract) EmitSetApprovalByCategoryEvent(owner, to, categoryName string, isApproval bool) {
	if isApproval {
		sdk.Instance.EmitEvent("SetApprovalByCategory", []string{owner, to, categoryName, standard.TrueString})
	} else {
		sdk.Instance.EmitEvent("SetApprovalByCategory", []string{owner, to, categoryName, standard.FalseString})
	}
}

// EmitCreateOrSetCategoryEvent emits CreateOrSetCategory event
func (c *CMNFAContract) EmitCreateOrSetCategoryEvent(categoryName, categoryURI string) {
	sdk.Instance.EmitEvent("CreateOrSetCategory", []string{categoryName, categoryURI})
}

func (c *CMNFAContract) setAccountToken(from, to string, tokenId string) error {
	am, err := c.getAccountStoreMap()
	if err != nil {
		return fmt.Errorf("new store map of token info failed, err:%s", err)
	}

	err = am.Set([]string{to, tokenId}, []byte(standard.TrueString))
	if err != nil {
		return fmt.Errorf("setAccountToken failed, err:%s", err)
	}
	if address.IsZeroAddress(from) {
		return nil
	}
	err = am.Del([]string{from, tokenId})
	if err != nil {
		return err
	}
	return nil
}

func (c *CMNFAContract) setTokenInfo(tokenId, categoryName string, metadata []byte) error {
	ti, err := c.getTokenInfoStoreMap()
	if err != nil {
		return fmt.Errorf("new store map of token info failed, err:%s", err)
	}
	err = ti.Set([]string{tokenId, categoryNameStoreKey}, []byte(categoryName))
	if err != nil {
		return fmt.Errorf("set category name of token info failed, err:%s", err)
	}
	if len(metadata) > 0 {
		err = ti.Set([]string{tokenId, metadataStoreKey}, metadata)
		if err != nil {
			return fmt.Errorf("set metadata of token info failed, err:%s", err)
		}
	}

	return nil
}

func (c *CMNFAContract) minted(tokenId string) (bool, error) {
	owner, err := c.OwnerOf(tokenId)
	if err != nil {
		return false, err
	}

	return address.IsValidAddress(owner) && !address.IsZeroAddress(owner), nil
}

func normalizeToHex(addr string) (string, error) {
    // 如果本身就是合法 hex
    if _, err := hex.DecodeString(addr); err == nil && len(addr) == 40 {
        return addr, nil
    }

    // 否则按 base58 解码
    decoded := base58.Decode(addr)
    if len(decoded) < 20 {
        return "", fmt.Errorf("invalid address: %s", addr)
    }

    raw20 := decoded[len(decoded)-20:]
    return hex.EncodeToString(raw20), nil
}


func (c *CMNFAContract) isApprovedOrOwner(sender string, tokenId string) (bool, error) {
	// check owner
	owner, err := c.OwnerOf(tokenId)
	if err != nil {
		return false, err
	}

    ownerHex, err := normalizeToHex(owner)
    if err != nil {
        return false, err
    }
    senderHex, err := normalizeToHex(sender)
    if err != nil {
        return false, err
    }

    if ownerHex == senderHex {
        return true, nil
    }

	// check tokenApprove
	approved, err := c.getApproved(tokenId)
	if err != nil {
		return false, err
	}
	if approved {
		return true, nil
	}
	approved, err = c.isApprovedForAll(owner, sender)
	if err != nil {
		return false, err
	}
	if approved {
		return true, nil
	}
	categoryName, err := c.getCategoryNameByTokenId(tokenId)
	if err != nil {
		return false, err
	}
	approved, err = c.isApprovedByCategory(owner, sender, categoryName)
	if err != nil {
		return false, err
	}

	return approved, nil
}

func (c *CMNFAContract) getBalance(balanceInfo *sdk.StoreMap, account string) (balance *safemath.SafeUint256,
	err error) {
	balanceBytes, err := balanceInfo.Get([]string{account})
	if err != nil {
		return nil, fmt.Errorf("get balance failed, err:%s", err)
	}
	balance, ok := safemath.ParseSafeUint256(string(balanceBytes))
	if !ok {
		return nil, fmt.Errorf("balance bytes invalid")
	}

	return balance, nil
}

func (c *CMNFAContract) increaseBalanceByOne(account string) error {
	balanceInfo, err := c.getBalanceInfoStoreMap()
	if err != nil {
		return fmt.Errorf("new storeMap of balanceInfo failed, err:%s", err)
	}
	originTokenCount, err := c.getBalance(balanceInfo, account)
	if err != nil {
		return fmt.Errorf("get token count failed, err:%s", err)
	}
	newTokenCount, ok := safemath.SafeAdd(originTokenCount, safemath.SafeUintOne)
	if !ok {
		return fmt.Errorf("balance count of from is overflow")
	}
	err = balanceInfo.Set([]string{account}, []byte(newTokenCount.ToString()))
	if err != nil {
		return fmt.Errorf(err.Error())
	}
	return nil
}

func (c *CMNFAContract) increaseTotalSupplyByOne() error {
	currentTotalSupplyStr, _, err := sdk.Instance.GetStateFromKeyWithExists(totalSupplyStoreKey)
	if err != nil {
		return err
	}
	currentTotalSupply, ok := safemath.ParseSafeUint256(currentTotalSupplyStr)
	if !ok {
		return fmt.Errorf("parse current total supply failed")
	}
	newTotalSupply, ok := safemath.SafeAdd(currentTotalSupply, safemath.SafeUintOne)
	if !ok {
		return fmt.Errorf("total supply too big")
	}
	err = sdk.Instance.PutStateFromKey(totalSupplyStoreKey, newTotalSupply.ToString())
	if err != nil {
		return err
	}
	return nil
}

func (c *CMNFAContract) increaseTotalSupplyOfCategoryByOne(categoryName string) error {
	totalSupplyOfCategoryMap, err := c.getCategoryTotalSupplyStoreMap()
	if err != nil {
		return err
	}
	currentTotalSupplyOfCategoryBytes, err := totalSupplyOfCategoryMap.Get([]string{categoryName})
	if err != nil {
		return err
	}
	currentTotalSupplyOfCategory, ok := safemath.ParseSafeUint256(string(currentTotalSupplyOfCategoryBytes))
	if !ok {
		return fmt.Errorf("invalid TotalSupply of category in store")
	}
	newTotalSupplyOfCategory, ok := safemath.SafeAdd(currentTotalSupplyOfCategory, safemath.SafeUintOne)
	if !ok {
		return fmt.Errorf("TotalSupply of category too big")
	}
	return totalSupplyOfCategoryMap.Set([]string{categoryName}, []byte(newTotalSupplyOfCategory.ToString()))
}

func (c *CMNFAContract) decreaseBalanceByOne(account string) error {
	balanceInfo, err := c.getBalanceInfoStoreMap()
	if err != nil {
		return fmt.Errorf("new storeMap of balanceInfo failed, err:%s", err)
	}
	originTokenCount, err := c.getBalance(balanceInfo, account)
	if err != nil {
		return fmt.Errorf("get token count failed, err:%s", err)
	}
	newTokenCount, ok := safemath.SafeSub(originTokenCount, safemath.SafeUintOne)
	if !ok {
		return fmt.Errorf("token count of account is overflow")
	}
	err = balanceInfo.Set([]string{account}, []byte(newTokenCount.ToString()))
	if err != nil {
		return fmt.Errorf(err.Error())
	}
	return nil
}

func (c *CMNFAContract) setTokenOwner(to string, tokenId string) error {
	tokenOwnerInfo, err := c.getTokenOwnerStoreMap()
	if err != nil {
		return fmt.Errorf("new storeMap of tokenOwner failed, err:%s", err)
	}
  	hexAddr, err := normalizeToHex(to)
	if err != nil {
    	return fmt.Errorf("invalid address for setTokenOwner: %s", err)
	}
	err = tokenOwnerInfo.Set([]string{tokenId}, []byte(hexAddr))

	if err != nil {
		return fmt.Errorf("set token owner failed, err:%s", err)
	}
	return nil
}

func (c *CMNFAContract) getTokenInfoStoreMap() (*sdk.StoreMap, error) {
	return sdk.NewStoreMap(tokenInfoMapName, 2, crypto.HASH_TYPE_SHA256)
}

func (c *CMNFAContract) getCategoryStoreMap() (*sdk.StoreMap, error) {
	return sdk.NewStoreMap(categoryMapName, 1, crypto.HASH_TYPE_SHA256)
}

func (c *CMNFAContract) getCategoryTotalSupplyStoreMap() (*sdk.StoreMap, error) {
	return sdk.NewStoreMap(categoryTotalSupplyMapName, 1, crypto.HASH_TYPE_SHA256)
}

func (c *CMNFAContract) getBalanceInfoStoreMap() (*sdk.StoreMap, error) {
	return sdk.NewStoreMap(balanceInfoMapName, 1, crypto.HASH_TYPE_SHA256)
}

func (c *CMNFAContract) getAccountStoreMap() (*sdk.StoreMap, error) {
	return sdk.NewStoreMap(accountMapName, 2, crypto.HASH_TYPE_SHA256)
}
func (c *CMNFAContract) getTokenOwnerStoreMap() (*sdk.StoreMap, error) {
	return sdk.NewStoreMap(tokenOwnerMapName, 1, crypto.HASH_TYPE_SHA256)
}
func (c *CMNFAContract) getTokenApprovalStoreMap() (*sdk.StoreMap, error) {
	return sdk.NewStoreMap(tokenApprovalMapName, 1, crypto.HASH_TYPE_SHA256)
}
func (c *CMNFAContract) getTokenApprovalForAllStoreMap() (*sdk.StoreMap, error) {
	return sdk.NewStoreMap(tokenApprovalForAllMapName, 2, crypto.HASH_TYPE_SHA256)
}
func (c *CMNFAContract) getTokenApprovalByCategoryStoreMap() (*sdk.StoreMap, error) {
	return sdk.NewStoreMap(tokenApprovalByCategoryMapName, 3, crypto.HASH_TYPE_SHA256)
}

func main() {
	err := sandbox.Start(new(CMNFAContract))
	if err != nil {
		sdk.Instance.Errorf(err.Error())
	}
}
